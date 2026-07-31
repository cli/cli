/**
 * @name HTTP response content reaches a terminal without ContentOut or sanitization
 * @description Raw bytes consumed from an HTTP response body, or reintroduced by
 *              decoding base64, must either be written to `IOStreams.ContentOut`
 *              or wrapped with the asciisanitizer before reaching a terminal
 *              writer. The body is tracked across function boundaries, so a body
 *              returned from a fetch helper and consumed by its caller is still
 *              covered. Values produced by structured decoding (encoding/json)
 *              are trusted, since cli/cli's REST clients sanitize JSON bodies at
 *              the transport layer before decoding.
 * @kind path-problem
 * @problem.severity error
 * @precision medium
 * @id cli-cli/unsanitized-response-to-terminal
 * @tags security
 */

import go
import semmle.go.dataflow.TaintTracking

// ContentOut is the blessed sanitizing writer. Writing raw content there is the
// safe choice, so it is excluded from the terminal sink set below.
predicate isContentOutRead(DataFlow::Node n) {
  exists(Field f |
    f.hasQualifiedName("github.com/cli/cli/v2/pkg/iostreams", "IOStreams", "ContentOut") and
    n = f.getARead()
  )
}

// Value flow from a ContentOut read so a ContentOut writer stored in a local
// variable is still recognised as the blessed sink. Plain DataFlow (not taint)
// is used so it does not leak across sibling fields of a shared IOStreams.
module ContentOutWriterConfig implements DataFlow::ConfigSig {
  predicate isSource(DataFlow::Node n) { isContentOutRead(n) }

  predicate isSink(DataFlow::Node n) { exists(n) }
}

module ContentOutWriterFlow = DataFlow::Global<ContentOutWriterConfig>;

predicate isContentOutWriter(DataFlow::Node n) {
  isContentOutRead(n) or
  exists(DataFlow::Node src | isContentOutRead(src) and ContentOutWriterFlow::flow(src, n))
}

// ANSI injection requires bytes to reach a terminal. The terminal-bound writers
// are os.Stdout / os.Stderr and the IOStreams.Out / IOStreams.ErrOut fields.
// File, socket, and buffer writers are not terminals and are intentionally out
// of scope.
predicate isTerminalWriterRead(DataFlow::Node n) {
  (
    exists(Variable v |
      v.hasQualifiedName("os", "Stdout") or v.hasQualifiedName("os", "Stderr")
    |
      n = v.getARead()
    )
    or
    exists(Field f |
      f.hasQualifiedName("github.com/cli/cli/v2/pkg/iostreams", "IOStreams", "Out") or
      f.hasQualifiedName("github.com/cli/cli/v2/pkg/iostreams", "IOStreams", "ErrOut")
    |
      n = f.getARead()
    )
  ) and
  not isContentOutRead(n)
}

// Value flow from a terminal-writer read so aliased writers
// (`w := opts.IO.Out; fmt.Fprint(w, ...)`) still count as terminal sinks. Plain
// DataFlow (not taint) is used so it does not leak across sibling fields of a
// shared IOStreams (which would otherwise mark ContentOut as terminal-bound).
module TerminalWriterConfig implements DataFlow::ConfigSig {
  predicate isSource(DataFlow::Node n) { isTerminalWriterRead(n) }

  predicate isSink(DataFlow::Node n) { exists(n) }
}

module TerminalWriterFlow = DataFlow::Global<TerminalWriterConfig>;

predicate isTerminalBoundWriter(DataFlow::Node n) {
  isTerminalWriterRead(n) or
  exists(DataFlow::Node src | isTerminalWriterRead(src) and TerminalWriterFlow::flow(src, n))
}

// Raw HTTP body reader. Sourcing at the field read (rather than a local
// consumption call) lets global taint carry the reader across returns and
// parameters before anything reads it.
predicate isResponseBodyReader(DataFlow::Node n) {
  exists(Field bodyField |
    bodyField.hasQualifiedName("net/http", "Response", "Body") and
    n = bodyField.getARead()
  )
}

// Base64 decoding reintroduces raw bytes that any text sanitization applied to
// the encoded form never saw, so the decoded stream is its own source.
predicate isBase64DecodeSource(DataFlow::Node n) {
  exists(DataFlow::CallNode c |
    c.getTarget().hasQualifiedName("encoding/base64", "NewDecoder") and n = c
  )
  or
  exists(DataFlow::MethodCallNode c |
    c.getTarget().hasQualifiedName("encoding/base64", "Encoding", "DecodeString") and n = c
  )
}

// Carry taint from a reader value into the bytes a read produces, so the flow
// continues from the reader to wherever those bytes are written.
predicate isReaderConsumptionStep(DataFlow::Node pred, DataFlow::Node succ) {
  exists(DataFlow::CallNode call |
    (
      call.getTarget().hasQualifiedName("io", "ReadAll") or
      call.getTarget().hasQualifiedName("io/ioutil", "ReadAll")
    ) and
    pred = call.getArgument(0) and
    // ReadAll returns (bytes, error). Track taint into result 0, the bytes, only.
    // The error result is an I/O or decode failure string, never response data,
    // so tracking it would flag code that merely prints that error.
    succ = call.getResult(0)
  )
  or
  exists(DataFlow::CallNode call |
    call.getTarget().hasQualifiedName("bufio", "NewScanner") and
    pred = call.getArgument(0) and
    succ = call
  )
  or
  exists(DataFlow::MethodCallNode read |
    (
      read.getTarget().hasQualifiedName("bufio", "Scanner", "Text") or
      read.getTarget().hasQualifiedName("bufio", "Scanner", "Bytes")
    ) and
    pred = read.getReceiver() and
    succ = read
  )
}

// The asciisanitizer wrap is the blessed barrier. Both Sanitizer{} and
// &Sanitizer{} forms are accepted, regardless of which fields are set.
predicate isSanitizerBarrier(DataFlow::Node n) {
  exists(DataFlow::CallNode c, Type argTy |
    c.getTarget().hasQualifiedName("golang.org/x/text/transform", "NewReader") and
    argTy = c.getArgument(1).getType() and
    (
      argTy.hasQualifiedName("github.com/cli/go-gh/v2/pkg/asciisanitizer", "Sanitizer") or
      argTy
          .(PointerType)
          .getBaseType()
          .hasQualifiedName("github.com/cli/go-gh/v2/pkg/asciisanitizer", "Sanitizer")
    ) and
    n = c
  )
}

// Structured decoding is trusted. Both `json.Unmarshal` and
// `(*json.Decoder).Decode` go through cli/cli's REST clients, which are built on
// go-gh's sanitizing transport: for JSON content types the body is sanitized
// before any decode. A decoded value is therefore not raw external content.
predicate isStructuredDecodeBarrier(DataFlow::Node n) {
  exists(DataFlow::CallNode c |
    c.getTarget().hasQualifiedName("encoding/json", "Unmarshal") and
    n = c.getArgument(0)
  )
  or
  exists(DataFlow::MethodCallNode c |
    c.getTarget().hasQualifiedName("encoding/json", "Decoder", "Decode") and
    n = c.getReceiver()
  )
}

// iostreams.Untrusted.String() returns content with ANSI escapes neutralized, so
// its result is sanitized. Raw and RawBytes are the deliberate opt-out and are
// intentionally NOT barriers, so a value taken out through them stays tracked to
// the terminal (unless the destination is ContentOut, which sanitizes itself).
predicate isUntrustedStringBarrier(DataFlow::Node n) {
  exists(DataFlow::MethodCallNode c |
    c.getTarget().hasQualifiedName("github.com/cli/cli/v2/pkg/iostreams", "Untrusted", "String") and
    n = c
  )
}

module UnsanitizedResponseConfig implements DataFlow::ConfigSig {
  predicate isSource(DataFlow::Node n) {
    isResponseBodyReader(n) or isBase64DecodeSource(n)
  }

  predicate isSink(DataFlow::Node n) {
    exists(DataFlow::CallNode call, int i |
      (
        call.getTarget().hasQualifiedName("fmt", "Fprint") or
        call.getTarget().hasQualifiedName("fmt", "Fprintln") or
        call.getTarget().hasQualifiedName("fmt", "Fprintf")
      ) and
      isTerminalBoundWriter(call.getArgument(0)) and
      not isContentOutWriter(call.getArgument(0)) and
      i >= 1 and
      n = call.getArgument(i)
    )
    or
    exists(DataFlow::CallNode call |
      (
        call.getTarget().hasQualifiedName("io", "Copy") or
        call.getTarget().hasQualifiedName("io", "CopyBuffer")
      ) and
      isTerminalBoundWriter(call.getArgument(0)) and
      not isContentOutWriter(call.getArgument(0)) and
      n = call.getArgument(1)
    )
    or
    exists(DataFlow::MethodCallNode call |
      call.getTarget().getName() = "Write" and
      isTerminalBoundWriter(call.getReceiver()) and
      not isContentOutWriter(call.getReceiver()) and
      n = call.getArgument(0)
    )
  }

  predicate isBarrier(DataFlow::Node n) {
    isSanitizerBarrier(n) or isStructuredDecodeBarrier(n) or isUntrustedStringBarrier(n)
  }

  predicate isAdditionalFlowStep(DataFlow::Node pred, DataFlow::Node succ) {
    isReaderConsumptionStep(pred, succ)
  }
}

module UnsanitizedResponseFlow = TaintTracking::Global<UnsanitizedResponseConfig>;

import UnsanitizedResponseFlow::PathGraph

from UnsanitizedResponseFlow::PathNode source, UnsanitizedResponseFlow::PathNode sink
where
  UnsanitizedResponseFlow::flowPath(source, sink) and
  not sink.getNode().getFile().getRelativePath().regexpMatch(".*_test\\.go") and
  not sink.getNode().getFile().getRelativePath().regexpMatch("internal/fake_vuln/.*") and
  not sink.getNode().getFile().getRelativePath().regexpMatch("\\.github/codeql/tests/.*")
select sink.getNode(), source, sink,
  "HTTP response content reaches a terminal writer that is not IOStreams.ContentOut. " +
  "Write external content to opts.IO.ContentOut, or wrap it with the asciisanitizer."
