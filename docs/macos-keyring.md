# macOS Keyring Security

This document describes how the GitHub CLI uses the macOS keyring, and related security notes.

The GitHub CLI updates the keyring on the following commands:
 * `gh auth login` (store a new token)
 * `gh auth refresh` (store a new token)
 * `gh auth logout` (delete a token)
 * `gh auth switch` (swap the token for the "active" account)

Additionally, it reads from the keyring on any command that requires a token.

### Implementation

Keyring support is provided by the [zalando/go-keyring](https://github.com/zalando/go-keyring) module. On MacOS, this [execs the `/usr/bin/security` binary](https://github.com/zalando/go-keyring/blob/v0.2.8/keyring_darwin.go#L44-L48) to interact with the keyring. Alternatives to this approach involve `cgo`, as per [ByteNess/go-keychain](https://github.com/ByteNess/go-keychain/blob/c96c38f7f906df0da922f9422a049c550084ce9f/keychain.go#L10) or `purego` as per [lstoll/keychain](https://github.com/lstoll/keychain). Choosing between these options has tradeoffs as described below.

### Consequences of using `/usr/bin/security`

Access to keyring items is protected via an ACL (Access Control List). When an application attempts to access an item, the user is prompted to `allow`, `deny`, or `always allow`. In the case of `always allow`, this decision is persisted for future access attempts by the same application.

Since the binary accessing the `gh` keyring items is `/usr/bin/security`, calling `security` directly from a terminal can also access the stored `gh` tokens for a given host and user.

Historically, this has not been a significant concern with downsides to the alternatives (see below), primarily because `gh` offers direct access to the token via `gh auth token`. There is an argument to be made that `gh` should be less _surprising_ in this behaviour, and with the rise in agentic development, this probably has more merit.

### Consequences of using Apple native APIs

Aside from the increased maintenance burden of introducing `cgo` or `purego`, there is a significant consequence for Homebrew distributions. Trusted applications in the keyring ACL are identified differently depending on whether they are codesigned with a stable identity. For an application signed with a certificate (e.g. a Developer ID), the identity remains stable across artifacts signed by the same certificate. For an application without a stable signing identity - either unsigned, or ad-hoc signed as the Go toolchain does for Apple Silicon binaries - the identity is determined by a hash of the code bytes (its `cdhash`). Therefore, for such applications, any change invalidates the keyring ACL.

Our primary distribution mechanism for MacOS is `homebrew`, which [builds `gh` as part of the packaging/installation pipeline](https://github.com/Homebrew/homebrew-core/blob/922e24021672a1627218fe4bb2af67bcd20d86df/Formula/g/gh.rb#L42), resulting in an artifact without a stable signing identity (unsigned on x86_64, ad-hoc signed on Apple Silicon). The consequence of this is that on every upgrade of the homebrew package, users will be prompted for keyring access.

This may additionally affect anyone else building for MacOS either for distribution (e.g. `conda`, `flox`, `macports`, `spack`, `webi`) or personally (e.g. `go install`).

For homebrew, resolving this would involve moving `gh` to a `cask` distribution, where `gh` builds and signs the application. It's not clear the full consequences of moving from a formula to a cask, but homebrew maintainers recommend formulas where possible.

### Conclusion

The GitHub CLI interacts with the keyring in a surprising manner, leaving a narrow (but increasing, due to agent adoption) opportunity for a token to be obtained unexpectedly. As maintainers, we currently believe the convenience for distributors and users outweighs the risks proposed.