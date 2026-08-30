/**
 * @name HTTP request URL not built with safeurl.SafeURL
 * @description Flags any HTTP request, a REST API call being the common case, whose URL argument is
 *              not literally a call to (safeurl.SafeURL).String. The argument expression itself must
 *              be a SafeURL.String call; any other form, such as a string literal, string
 *              concatenation, or fmt.Sprintf, is reported. This keeps every hand built URL routed
 *              through safeurl so its variable path components are percent-encoded.
 * @kind problem
 * @problem.severity warning
 * @precision high
 * @id cli-cli/safeurl-path-construction
 * @tags security
 *       correctness
 *       maintainability
 */

import go

/**
 * Holds when `node` is the URL argument of an HTTP request, a REST API call being the common case.
 *
 * Covered entry points:
 *   - (github.com/cli/cli/v2/api.Client).REST and .RESTWithNext, where the path is argument 2.
 *   - net/http.NewRequest, where the URL is argument 1.
 *   - net/http.NewRequestWithContext, where the URL is argument 2.
 *   - (net/http.Client).Get, .Head, .Post and .PostForm, where the URL is argument 0.
 */
predicate isHttpUrlArgument(DataFlow::Node node) {
  exists(Method m, DataFlow::CallNode call |
    m.hasQualifiedName("github.com/cli/cli/v2/api", "Client", ["REST", "RESTWithNext"]) and
    call = m.getACall() and
    node = call.getArgument(2)
  )
  or
  exists(Function f, DataFlow::CallNode call |
    f.hasQualifiedName("net/http", "NewRequest") and
    call = f.getACall() and
    node = call.getArgument(1)
  )
  or
  exists(Function f, DataFlow::CallNode call |
    f.hasQualifiedName("net/http", "NewRequestWithContext") and
    call = f.getACall() and
    node = call.getArgument(2)
  )
  or
  exists(Method m, DataFlow::CallNode call |
    m.hasQualifiedName("net/http", "Client", ["Get", "Head", "Post", "PostForm"]) and
    call = m.getACall() and
    node = call.getArgument(0)
  )
}

/**
 * Holds when `node` is a call to the String method of one of the safeurl URL types:
 * the SafeURL interface or either of its implementations, MutableSafeURL and
 * ImmutableSafeURL. Matching all three keeps call sites free of explicit conversions:
 * a value of the concrete type can be passed to the sink directly without first being
 * assigned to a SafeURL typed variable.
 */
predicate isSafeurlStringCall(DataFlow::Node node) {
  exists(Method m |
    m.hasQualifiedName("github.com/cli/cli/v2/internal/safeurl",
      ["SafeURL", "MutableSafeURL", "ImmutableSafeURL"], "String") and
    node = m.getACall()
  )
}

from DataFlow::Node sink
where
  isHttpUrlArgument(sink) and
  not isSafeurlStringCall(sink)
select sink, "This HTTP request URL is not passed directly as the result of safeurl.SafeURL.String."
