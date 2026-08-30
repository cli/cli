/**
 * @name ImmutableSafeURL built from a hand-assembled string
 * @description Flags a call to safeurl.NewImmutableSafeURL whose argument is a locally assembled
 *              string, that is a value tainted by fmt.Sprintf, fmt.Sprint, fmt.Sprintln or a string
 *              concatenation. NewImmutableSafeURL renders its argument verbatim, skipping the
 *              percent-encoding and traversal check that JoinPath applies, so it must only receive an
 *              already formed, trusted URL such as a server returned field or a pagination link. A
 *              hand-built path reaching it is a way to route around safeurl and must instead be built
 *              with safeurl.JoinPath. This query is a convention guard, it cannot and does not verify
 *              the trustedness of URLs read from struct fields or returned by API calls.
 * @kind problem
 * @problem.severity warning
 * @precision high
 * @id cli-cli/immutable-safeurl-construction
 * @tags security
 *       correctness
 *       maintainability
 */

import go

/**
 * Holds when `node` is the URL argument of a call to safeurl.NewImmutableSafeURL, the escape hatch
 * that renders its argument verbatim without percent-encoding or a traversal check.
 */
predicate isImmutableSafeURLArgument(DataFlow::Node node) {
  exists(Function f, DataFlow::CallNode call |
    f.hasQualifiedName("github.com/cli/cli/v2/internal/safeurl", "NewImmutableSafeURL") and
    call = f.getACall() and
    node = call.getArgument(0)
  )
}

/**
 * Holds when `node` is a locally assembled string: the result of fmt.Sprintf, fmt.Sprint or
 * fmt.Sprintln, or a string concatenation expression. These are the shapes that build a URL by hand
 * rather than reading an already formed value, so they must not reach NewImmutableSafeURL.
 */
predicate isHandAssembledString(DataFlow::Node node) {
  exists(Function f |
    f.hasQualifiedName("fmt", ["Sprintf", "Sprint", "Sprintln"]) and
    node = f.getACall()
  )
  or
  exists(AddExpr e |
    e.getType() instanceof StringType and
    node = DataFlow::exprNode(e)
  )
}

from DataFlow::Node source, DataFlow::Node sink
where
  isImmutableSafeURLArgument(sink) and
  isHandAssembledString(source) and
  TaintTracking::localTaint(source, sink)
select sink,
  "This ImmutableSafeURL is built from a hand-assembled string ($@); build the path with safeurl.JoinPath so its components are escaped and traversal-checked.",
  source, "assembled here"
