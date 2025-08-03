# Error Handling Guidance

Previously Boulder has used a mix of various error types to represent errors internally, mainly the `core.XXXError` types and `probs.ProblemDetails`, without any guidance on which should be used when or where.

We have switched away from this to using a single unified internal error type, `boulder/errors.BoulderError` which should be used anywhere we need to pass errors between components and need to be able to indicate and test the type of the error that was passed. `probs.ProblemDetails` should only be used in the WFE when creating a problem document to pass directly back to the user client.

A mapping exists in the WFE to map all of the available `boulder/errors.ErrorType`s to the relevant `probs.ProblemType`s. Internally errors should be wrapped when doing so provides some further context to the error that aides in debugging or will be passed back to the user client. An error may be unwrapped, or a simple stdlib `error` may be used, but doing so means the `probs.ProblemType` mapping will always be `probs.ServerInternalProblem` so should only be used for errors that do not need to be presented back to the user client.

`boulder/errors.BoulderError`s have two components: an internal type, `boulder/errors.ErrorType`, and a detail string. The internal type should be used for a. allowing the receiver to determine what caused the error, e.g. by using `boulder/errors.NotFound` to indicate a DB operation couldn't find the requested resource, and b. allowing the WFE to convert the error to the relevant `probs.ProblemType` for display to the user. The detail string should provide a user readable explanation of the issue to be presented to the user; the only exception to this is when the internal type is `boulder/errors.InternalServer` in which case the detail of the error will be stripped by the WFE and the only message presented to the user will be provided by the caller in the WFE.

Error type testing should be done with `boulder/errors.Is` instead of locally doing a type cast test. 
