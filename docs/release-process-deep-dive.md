# Release Process Deep Dive

The current release workflow and associated scripts were created before the current set of maintainers, and all maintainers from that time have left. On a number of occasions (releasing a MacOS installer, moving to Azure HSM signing, updating expired GPG key) the current maintainers have spent time investigating the release workflow. This document is intended to serve as a guide for future maintainers who need to understand the release process.

# High Level Overview

From a high level, the [release workflow](https://github.com/cli/cli/blob/537a22228cd6b42b740d7f1c09f47c45bb1dab30/.github/workflows/deployment.yml):
 * Is triggered by a `workflow_dispatch` event (typically a result of running `./script/release`)
 * Builds, packages and signs artifacts in parallel for Linux, MacOS and Windows
 * GPG signs Debian and Red Hat repository artifacts
 * Builds and updates the [manual](https://cli.github.com/manual) and repository packages
 * Creates GitHub Attestations for the artifacts
 * Creates a GitHub Release and attaches the artifacts
 * Bumps the `gh` [homebrew-core formula](https://github.com/Homebrew/homebrew-core/blob/2df031cbd8f7bc9b9a380e941ccefcf3c8f3d02b/Formula/g/gh.rb)

# Jobs Deep Dive

This section will deep dive into each job in the [`deployment.yml` workflow](https://github.com/cli/cli/blob/537a22228cd6b42b740d7f1c09f47c45bb1dab30/.github/workflows/deployment.yml).

- [validate-tag-name](#validate-tag-name)
- [OS Specific Builds](#os-builds)
  - [linux](#linux)
  - [macos](#macos)
  - [windows](#windows)
- [release](#release)

Although this workflow is used to do our production releases for Linux, MacOS and Windows, it is also possible to run subsets of the workflow. Specifically:
 * The workflow can be triggered with `inputs.release` set to `false`, resulting in the entire [release job](#release) being skipped. This is not exposed via `./script/release`.
 * Many sections are guarded by `if: inputs.environment == 'production'`. These guards protect sections that require secrets (e.g. signing) or that result in mutations (e.g. creating a GitHub release). `./script/release` accepts the `--staging` flag for this purpose. This differs from the previous bullet point as some steps in the [release job](#release) print debug information such as [`git` diffs](https://github.com/cli/cli/blob/5d2eadef8cccf2671f68aad05cd93215a4c01b48/.github/workflows/deployment.yml#L380-L384).
 * The workflow can be triggered for only `linux`, `MacOS` or `Windows` which allows for debugging single jobs. This is not exposed via `./script/release`. The [release job](#release) should not run in this case as it [requires all OS specific builds.](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.github/workflows/deployment.yml#L252)

## <a id="validate-tag-name">[validate-tag-name](https://github.com/cli/cli/blob/537a22228cd6b42b740d7f1c09f47c45bb1dab30/.github/workflows/deployment.yml#L31-L39)</a>

<details>

```yml
  validate-tag-name:
    runs-on: ubuntu-latest
    steps:
      - name: Validate tag name format
        run: |
          if [[ ! "${{ inputs.tag_name }}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo "Invalid tag name format. Must be in the form v1.2.3"
            exit 1
          fi
```
</details>

The purpose of this job is to [prevent incorrectly tagged releases](https://github.com/cli/cli/pull/10121), by ensuring they conform to the `major.minor.patch` form of semantic versioning, preceded by a `v`.

> [!WARNING]
> The `release` job can [create the GitHub release as a pre-release based on the existence of a hyphen in the tag name](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.github/workflows/deployment.yml#L362-L364), but the later addition of `validate-tag-name` disallows this.

## <a id="os-builds">[OS Builds](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.github/workflows/deployment.yml#L40-L248)</a>

After validating the tag name, the workflow parallelises across `ubuntu`, `macos` and `windows` runners. The primary purpose of these jobs is to build and sign release artifacts. These artifacts are made available to the `release` job via `actions/upload-artifact` and `actions/download-artifact` respectively.

### <a id="linux">[linux](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.github/workflows/deployment.yml#L40-L73)</a>

<details>

```yml
  linux:
    needs: validate-tag-name
    runs-on: ubuntu-latest
    environment: ${{ inputs.environment }}
    if: contains(inputs.platforms, 'linux')
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'
      - name: Install GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~1.17.1"
          install-only: true
      - name: Build release binaries
        env:
          TAG_NAME: ${{ inputs.tag_name }}
        run: script/release --local "$TAG_NAME" --platform linux
      - name: Generate web manual pages
        run: |
          go run ./cmd/gen-docs --website --doc-path dist/manual
          tar -czvf dist/manual.tar.gz -C dist -- manual
      - uses: actions/upload-artifact@v4
        with:
          name: linux
          if-no-files-found: error
          retention-days: 7
          path: |
            dist/*.tar.gz
            dist/*.rpm
            dist/*.deb
```
</details>

In addition to building release artifacts, the `linux` job builds the [CLI manual](https://cli.github.com/manual/) for use in the later `release` job.

#### Building

This job executes `script/release --local "$TAG_NAME" --platform linux` which uses`GoReleaser` to create the Go executables, `.zip` archives, and `.deb` / `.rpm` repository packages. See [how ./script/release works](#how-script-release-works) for further information.

#### Signing

There is no signing of linux artifacts in this job. See the [release job](#release) for more information on signing linux artifacts.

### <a id="macos">[macos](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.github/workflows/deployment.yml#L75-L145)</a>

<details>

```yaml
  macos:
    needs: validate-tag-name
    runs-on: macos-latest
    environment: ${{ inputs.environment }}
    if: contains(inputs.platforms, 'macos')
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'
      - name: Configure macOS signing
        if: inputs.environment == 'production'
        env:
          APPLE_DEVELOPER_ID: ${{ vars.APPLE_DEVELOPER_ID }}
          APPLE_APPLICATION_CERT: ${{ secrets.APPLE_APPLICATION_CERT }}
          APPLE_APPLICATION_CERT_PASSWORD: ${{ secrets.APPLE_APPLICATION_CERT_PASSWORD }}
        run: |
          keychain="$RUNNER_TEMP/buildagent.keychain"
          keychain_password="password1"

          security create-keychain -p "$keychain_password" "$keychain"
          security default-keychain -s "$keychain"
          security unlock-keychain -p "$keychain_password" "$keychain"

          base64 -D <<<"$APPLE_APPLICATION_CERT" > "$RUNNER_TEMP/cert.p12"
          security import "$RUNNER_TEMP/cert.p12" -k "$keychain" -P "$APPLE_APPLICATION_CERT_PASSWORD" -T /usr/bin/codesign
          security set-key-partition-list -S "apple-tool:,apple:,codesign:" -s -k "$keychain_password" "$keychain"
          rm "$RUNNER_TEMP/cert.p12"
      - name: Install GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~1.17.1"
          install-only: true
      - name: Build release binaries
        env:
          TAG_NAME: ${{ inputs.tag_name }}
          APPLE_DEVELOPER_ID: ${{ vars.APPLE_DEVELOPER_ID }}
        run: script/release --local "$TAG_NAME" --platform macos
      - name: Notarize macOS archives
        if: inputs.environment == 'production'
        env:
          APPLE_ID: ${{ vars.APPLE_ID }}
          APPLE_ID_PASSWORD: ${{ secrets.APPLE_ID_PASSWORD }}
          APPLE_DEVELOPER_ID: ${{ vars.APPLE_DEVELOPER_ID }}
        run: |
          shopt -s failglob
          script/sign dist/gh_*_macOS_*.zip
      - name: Build universal macOS pkg installer
        if: inputs.environment != 'production'
        env:
          TAG_NAME: ${{ inputs.tag_name }}
        run: script/pkgmacos "$TAG_NAME"
      - name: Build & notarize universal macOS pkg installer
        if: inputs.environment == 'production'
        env:
          TAG_NAME: ${{ inputs.tag_name }}
          APPLE_DEVELOPER_INSTALLER_ID: ${{ vars.APPLE_DEVELOPER_INSTALLER_ID }}
        run: |
          shopt -s failglob
          script/pkgmacos "$TAG_NAME"
      - uses: actions/upload-artifact@v4
        with:
          name: macos
          if-no-files-found: error
          retention-days: 7
          path: |
            dist/*.tar.gz
            dist/*.zip
            dist/*.pkg
```
</details>

#### Building

This job executes `script/release --local "$TAG_NAME" --platform macos` which uses `GoReleaser` to create the Go executables and `.zip` archives. See [how ./script/release works](#how-script-release-works) for further information.

This job also executes `script/pkgmacos "$TAG_NAME"` to build a Universal (architecture independent) MacOS `.pkg` installer. See [how ./script/pkgmacos works](#how-script-pkgmacos-works) for further information.

#### Signing

For MacOS, the "signing" section refers to both signing and notarizing.

There are three levels of "signing" that occur in this job:
 * Signing of Go executables created by `GoReleaser` is performed in a [`GoReleaser` hook](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.goreleaser.yml#L20).
 * Notarization of `.zip` archives created by `GoReleaser` is performed by directly executing `script/sign dist/gh_*_macOS_*.zip`
 * Signing of the `.pkg` installer in `./script/pkgmacos` when executing [`productbuild`](https://github.com/cli/cli/blob/1c74296d28cf5595008065d3ddf7061ca9388305/script/pkgmacos#L108). See warnings below.

 > [!WARNING]
> Although the job title is `Build & notarize universal macOS pkg installer`, the [`productbuild` docs](https://www.unix.com/man_page/osx/1/productbuild/) only refer to signing, thus notarization may not be the correct term here.

> [!WARNING]
> Although it appears as if signing the `.pkg` installer can occur if `inputs.environment == 'production'`, in practice, I don't believe we ever set `${{ vars.APPLE_DEVELOPER_INSTALLER_ID }}`, thus we always [skip signing](https://github.com/cli/cli/actions/runs/13271193192/job/37050749548#step:9:11).

Signing of MacOS artifacts uses `codesign` and notarization uses `xcrun notarytool`, which submits the artifact to the Apple servers for additional checks.

In order to perform signing, a keychain must be configured with the signing certificate. Comments have been added to provide clarity to the script:

```sh
keychain="$RUNNER_TEMP/buildagent.keychain"
keychain_password="password1"

# Create a new keychain for credentials to be stored in.
security create-keychain -p "$keychain_password" "$keychain"
# Mark the keychain as the system default so that a later signing step doesn't require
# referencing the keychain by name.
security default-keychain -s "$keychain"
# Unlock the keychain so that future operations can access the secrets without user interaction.
security unlock-keychain -p "$keychain_password" "$keychain"

base64 -D <<<"$APPLE_APPLICATION_CERT" > "$RUNNER_TEMP/cert.p12"

# Import the certificate into the keychain so that a later signing step can use it.
# `man security` snippet:
# -k keychain     Specify keychain into which item(s) will be imported.
# -P passphrase   Specify the unwrapping passphrase immediately. The default is to obtain a secure passphrase via GUI.
# -T appPath      Specify an application which may access the imported key (multiple -T options are allowed)
security import "$RUNNER_TEMP/cert.p12" -k "$keychain" -P "$APPLE_APPLICATION_CERT_PASSWORD" -T /usr/bin/codesign

# Enforce additional security requirements that only the applications used for signing can access the keychain. This allows for signing applications to access the keychain without user interaction.
# The three values:
#  * apple-tool: → Grants access to Apple's development tools.
#  * apple: → Grants access to Apple’s general cryptographic tools.
#  * codesign: → Grants access to the codesign tool, which is used to sign binaries and applications.
#
# `man security` snippet:
# set-key-partition-list [-S partition-list] [-k password] [options...] [keychain] Sets the "partition list" for a key. The "partition list" is an extra parameter in the ACL which limits access to the key based on an application's code signature. You must present the keychain's password to change a partition list. If you'd like to run /usr/bin/codesign with the key, "apple:" must be an element of the partition
# list.

#       -S partition-list
#                       Comma-separated partition list. See output of "security dump-keychain" for examples.
#       -k password     Password for keychain
#       -s              Match keys that can sign
security set-key-partition-list -S "apple-tool:,apple:,codesign:" -s -k "$keychain_password" "$keychain"
# Clean up the certificate so that it's not lying around for later jobs to leak.
rm "$RUNNER_TEMP/cert.p12"
```

When we execute `codesign --timestamp --options=runtime -s "${APPLE_DEVELOPER_ID?}" -v "$1"` in `./script/sign`, `codesign` inspects into the default keychain to find a certificate that matches the `APPLE_DEVELOPER_ID` environment variable. The `--timestamp` and `--options=runtime` flags are required for Notarization, described below.

---

[Code signing certifies that a `gh` executable was created by GitHub](https://developer.apple.com/documentation/security/code-signing-services). On the other hand, [Notarization](https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution) is an additional security step upon which software is submitted to Apple for automated scanning. If passed, Apple generates a `ticket` that can be `stapled` to the software, and Apple's [Gatekeeper](https://support.apple.com/en-gb/guide/security/sec5599b66df/web) software is made aware of it.

When we execute `xcrun notarytool submit "$1" --apple-id "${APPLE_ID?}" --team-id "${APPLE_DEVELOPER_ID?}" --password "${APPLE_ID_PASSWORD?}"` in `./script/sign` no keychain access should be required as the `APPLE_ID_PASSWORD` environment variable is used to authenticate.

### <a id="windows">[windows](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.github/workflows/deployment.yml#L147-L248)</a>

<details>

```yml
windows:
    needs: validate-tag-name
    runs-on: windows-latest
    environment: ${{ inputs.environment }}
    if: contains(inputs.platforms, 'windows')
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version-file: 'go.mod'
      - name: Install GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: "~1.17.1"
          install-only: true
      - name: Install Azure Code Signing Client
        shell: pwsh
        env:
          ACS_DIR: ${{ runner.temp }}\acs
          ACS_ZIP: ${{ runner.temp }}\acs.zip
          CORRELATION_ID: ${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}
          METADATA_PATH: ${{ runner.temp }}\acs\metadata.json
        run: |
          # Download Azure Code Signing client containing the DLL needed for signtool in script/sign
          Invoke-WebRequest -Uri https://www.nuget.org/api/v2/package/Azure.CodeSigning.Client/1.0.43 -OutFile $Env:ACS_ZIP -Verbose
          Expand-Archive $Env:ACS_ZIP -Destination $Env:ACS_DIR -Force -Verbose

          # Generate metadata file for signtool, used in signing box .exe and .msi
          @{
            CertificateProfileName = "GitHubInc"
            CodeSigningAccountName = "GitHubInc"
            CorrelationId = $Env:CORRELATION_ID
            Endpoint =  "https://wus.codesigning.azure.net/"
          } | ConvertTo-Json | Out-File -FilePath $Env:METADATA_PATH

      # Azure Code Signing leverages the environment variables for secrets that complement the metadata.json
      # file generated above (AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID)
      # For more information, see https://learn.microsoft.com/en-us/dotnet/api/azure.identity.defaultazurecredential?view=azure-dotnet
      - name: Build release binaries
        shell: bash
        env:
          AZURE_CLIENT_ID: ${{ secrets.SPN_GITHUB_CLI_SIGNING_CLIENT_ID }}
          AZURE_CLIENT_SECRET: ${{ secrets.SPN_GITHUB_CLI_SIGNING }}
          AZURE_TENANT_ID: ${{ secrets.SPN_GITHUB_CLI_SIGNING_TENANT_ID }}
          DLIB_PATH: ${{ runner.temp }}\acs\bin\x64\Azure.CodeSigning.Dlib.dll
          METADATA_PATH: ${{ runner.temp }}\acs\metadata.json
          TAG_NAME: ${{ inputs.tag_name }}
        run: script/release --local "$TAG_NAME" --platform windows
      - name: Set up MSBuild
        id: setupmsbuild
        uses: microsoft/setup-msbuild@v2.0.0
      - name: Build MSI
        shell: bash
        env:
          MSBUILD_PATH: ${{ steps.setupmsbuild.outputs.msbuildPath }}
        run: |
          for ZIP_FILE in dist/gh_*_windows_*.zip; do
            MSI_NAME="$(basename "$ZIP_FILE" ".zip")"
            MSI_VERSION="$(cut -d_ -f2 <<<"$MSI_NAME" | cut -d- -f1)"
            case "$MSI_NAME" in
            *_386 )
              source_dir="$PWD/dist/windows_windows_386"
              platform="x86"
              ;;
            *_amd64 )
              source_dir="$PWD/dist/windows_windows_amd64_v1"
              platform="x64"
              ;;
            *_arm64 )
              source_dir="$PWD/dist/windows_windows_arm64"
              platform="arm64"
              ;;
            * )
              printf "unsupported architecture: %s\n" "$MSI_NAME" >&2
              exit 1
              ;;
            esac
            "${MSBUILD_PATH}\MSBuild.exe" ./build/windows/gh.wixproj -p:SourceDir="$source_dir" -p:OutputPath="$PWD/dist" -p:OutputName="$MSI_NAME" -p:ProductVersion="${MSI_VERSION#v}" -p:Platform="$platform"
          done
      - name: Sign .msi release binaries
        if: inputs.environment == 'production'
        shell: pwsh
        env:
          AZURE_CLIENT_ID: ${{ secrets.SPN_GITHUB_CLI_SIGNING_CLIENT_ID }}
          AZURE_CLIENT_SECRET: ${{ secrets.SPN_GITHUB_CLI_SIGNING }}
          AZURE_TENANT_ID: ${{ secrets.SPN_GITHUB_CLI_SIGNING_TENANT_ID }}
          DLIB_PATH: ${{ runner.temp }}\acs\bin\x64\Azure.CodeSigning.Dlib.dll
          METADATA_PATH: ${{ runner.temp }}\acs\metadata.json
        run: |
          Get-ChildItem -Path .\dist -Filter *.msi | ForEach-Object {
            .\script\sign.ps1 $_.FullName
          }
      - uses: actions/upload-artifact@v4
        with:
          name: windows
          if-no-files-found: error
          retention-days: 7
          path: |
            dist/*.zip
            dist/*.msi
```
</details>

#### Building

This job executes `script/release --local "$TAG_NAME" --platform windows` to use `GoReleaser` to create the Go executables and `.zip` archives. See [how ./script/release works](#how-script-release-works) for further information.

This job also executes `MSBuild.exe` to build MSI Installers, wrapping each architecture dependent `.zip` produced by GoReleaser. This is done via the command:

```pwsh
"${MSBUILD_PATH}\MSBuild.exe" ./build/windows/gh.wixproj -p:SourceDir="$source_dir" -p:OutputPath="$PWD/dist" -p:OutputName="$MSI_NAME" -p:ProductVersion="${MSI_VERSION#v}" -p:Platform="$platform"
```

This references a number of [pretty inscrutable files](https://github.com/cli/cli/tree/817eeb26e567de11007c8a82c25e61c7e20e4337/build/windows) in our repository that form a kind of manifest. Some of the details and motivation for the contents of these files is described in the [PR](https://github.com/cli/cli/pull/4276) that introduced them.

#### Signing

There are two levels of signing that occur in this job:
 * Signing of Go executables created by `GoReleaser` is performed in a [`GoReleaser` hook](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.goreleaser.yml#L43).
 * Signing of the MSI installers by executing `.\script\sign.ps1 $_.FullName`

Signing of the Windows artifacts uses `signtool.exe` to request signing from [Azure HSM](https://azure.microsoft.com/en-us/products/azure-dedicated-hsm). This takes the following steps:

Firstly, a package is downloaded that contains a DLL to allow `signtool.exe` to interact with Azure HSM.

```pwsh
# Download Azure Code Signing client containing the DLL needed for signtool in script/sign
Invoke-WebRequest -Uri https://www.nuget.org/api/v2/package/Azure.CodeSigning.Client/1.0.43 -OutFile $Env:ACS_ZIP -Verbose
Expand-Archive $Env:ACS_ZIP -Destination $Env:ACS_DIR -Force -Verbose
```

Secondly, we create a JSON file containing metadata required by HSM:

```pwsh
# Generate metadata file for signtool, used in signing box .exe and .msi
@{
  CertificateProfileName = "GitHubInc"
  CodeSigningAccountName = "GitHubInc"
  CorrelationId = $Env:CORRELATION_ID
  Endpoint =  "https://wus.codesigning.azure.net/"
} | ConvertTo-Json | Out-File -FilePath $Env:METADATA_PATH
```

Thirdly, in `./script/sign.ps1` we look for the `signtool` executable:

```pwsh
$signtool = Resolve-Path "C:\Program Files (x86)\Windows Kits\10\bin\*\x64\signtool.exe" | Select-Object -Last 1
```

Finally, in `./script/sign.ps`, we execute `signtool`:

```pwsh
& $signtool sign /d "GitHub CLI" /fd sha256 /td sha256 /tr http://timestamp.acs.microsoft.com /v /dlib "$Env:DLIB_PATH" /dmdf "$Env:METADATA_PATH" $Args[0]
```

Breaking this command down:
 * `/fd` is the file digest algorithm
 * `/td` is the timestamp digest algorithm
 * `/tr` indicates the timestamp server so a timestamp can be added to the signature, proving when it was signed
 * `/dlib` points to the previously extracted DLL
 * `/dmdf` points to the previously created metadata file

## <a id="release">[release](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.github/workflows/deployment.yml#L250-L395)</a>

<details>

```yml
release:
    runs-on: ubuntu-latest
    needs: [linux, macos, windows]
    environment: ${{ inputs.environment }}
    if: inputs.release
    steps:
      - name: Checkout cli/cli
        uses: actions/checkout@v4
      - name: Merge built artifacts
        uses: actions/download-artifact@v4
      - name: Checkout documentation site
        uses: actions/checkout@v4
        with:
          repository: github/cli.github.com
          path: site
          fetch-depth: 0
          token: ${{ secrets.SITE_DEPLOY_PAT }}
      - name: Update site man pages
        env:
          GIT_COMMITTER_NAME: cli automation
          GIT_AUTHOR_NAME: cli automation
          GIT_COMMITTER_EMAIL: noreply@github.com
          GIT_AUTHOR_EMAIL: noreply@github.com
          TAG_NAME: ${{ inputs.tag_name }}
        run: |
          git -C site rm 'manual/gh*.md' 2>/dev/null || true
          tar -xzvf linux/manual.tar.gz -C site
          git -C site add 'manual/gh*.md'
          sed -i.bak -E "s/(assign version = )\".+\"/\1\"${TAG_NAME#v}\"/" site/index.html
          rm -f site/index.html.bak
          git -C site add index.html
          git -C site diff --quiet --cached || git -C site commit -m "gh ${TAG_NAME#v}"
      - name: Prepare release assets
        env:
          TAG_NAME: ${{ inputs.tag_name }}
        run: |
          shopt -s failglob
          rm -rf dist
          mkdir dist
          mv -v {linux,macos,windows}/gh_* dist/
      - name: Install packaging dependencies
        run: sudo apt-get install -y rpm reprepro
      - name: Set up GPG
        if: inputs.environment == 'production'
        env:
          GPG_PUBKEY: ${{ secrets.GPG_PUBKEY }}
          GPG_KEY: ${{ secrets.GPG_KEY }}
          GPG_PASSPHRASE: ${{ secrets.GPG_PASSPHRASE }}
          GPG_KEYGRIP: ${{ secrets.GPG_KEYGRIP }}
        run: |
          base64 -d <<<"$GPG_PUBKEY" | gpg --import --no-tty --batch --yes
          base64 -d <<<"$GPG_KEY" | gpg --import --no-tty --batch --yes
          echo "allow-preset-passphrase" > ~/.gnupg/gpg-agent.conf
          gpg-connect-agent RELOADAGENT /bye
          /usr/lib/gnupg2/gpg-preset-passphrase --preset "$GPG_KEYGRIP" <<<"$GPG_PASSPHRASE"
      - name: Sign RPMs
        if: inputs.environment == 'production'
        run: |
          cp script/rpmmacros ~/.rpmmacros
          rpmsign --addsign dist/*.rpm
      - name: Attest release artifacts
        if: inputs.environment == 'production'
        uses: actions/attest-build-provenance@520d128f165991a6c774bcb264f323e3d70747f4 # v2.2.0
        with:
          subject-path: "dist/gh_*"
      - name: Run createrepo
        env:
          GPG_SIGN: ${{ inputs.environment == 'production' }}
        run: |
          mkdir -p site/packages/rpm
          cp dist/*.rpm site/packages/rpm/
          ./script/createrepo.sh
          cp -r dist/repodata site/packages/rpm/
          pushd site/packages/rpm
          [ "$GPG_SIGN" = "false" ] || gpg --yes --detach-sign --armor repodata/repomd.xml
          popd
      - name: Run reprepro
        env:
          GPG_SIGN: ${{ inputs.environment == 'production' }}
          # We are no longer adding to the distribution list.
          # All apt distributions should use "stable" according to our install documentation.
          # In the future we will remove legacy distributions listed here.
          RELEASES: "cosmic eoan disco groovy focal stable oldstable testing sid unstable buster bullseye stretch jessie bionic trusty precise xenial hirsute impish kali-rolling"
        run: |
          mkdir -p upload
          [ "$GPG_SIGN" = "true" ] || sed -i.bak '/^SignWith:/d' script/distributions
          for release in $RELEASES; do
            for file in dist/*.deb; do
              reprepro --confdir="+b/script" includedeb "$release" "$file"
            done
          done
          cp -a dists/ pool/ upload/
          mkdir -p site/packages
          cp -a upload/* site/packages/
      - name: Create the release
        env:
          # In non-production environments, the assets will not have been signed
          DO_PUBLISH: ${{ inputs.environment == 'production' }}
          TAG_NAME: ${{ inputs.tag_name }}
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          shopt -s failglob
          pushd dist
          shasum -a 256 gh_* > checksums.txt
          mv checksums.txt gh_${TAG_NAME#v}_checksums.txt
          popd
          release_args=(
            "$TAG_NAME"
            --title "GitHub CLI ${TAG_NAME#v}"
            --target "$GITHUB_SHA"
            --generate-notes
          )
          if [[ $TAG_NAME == *-* ]]; then
            release_args+=( --prerelease )
          fi
          guard="echo"
          [ "$DO_PUBLISH" = "false" ] || guard=""
          script/label-assets dist/gh_* | xargs $guard gh release create "${release_args[@]}" --
      - name: Publish site
        env:
          DO_PUBLISH: ${{ inputs.environment == 'production' && !contains(inputs.tag_name, '-') }}
          TAG_NAME: ${{ inputs.tag_name }}
          GIT_COMMITTER_NAME: cli automation
          GIT_AUTHOR_NAME: cli automation
          GIT_COMMITTER_EMAIL: noreply@github.com
          GIT_AUTHOR_EMAIL: noreply@github.com
        working-directory: ./site
        run: |
          git add packages
          git commit -m "Add rpm and deb packages for $TAG_NAME"
          if [ "$DO_PUBLISH" = "true" ]; then
            git push
          else
            git log --oneline @{upstream}..
            git diff --name-status @{upstream}..
          fi
      - name: Bump homebrew-core formula
        uses: mislav/bump-homebrew-formula-action@v3
        if: inputs.environment == 'production' && !contains(inputs.tag_name, '-')
        with:
          formula-name: gh
          formula-path: Formula/g/gh.rb
          tag-name: ${{ inputs.tag_name }}
          push-to: williammartin/homebrew-core
        env:
          COMMITTER_TOKEN: ${{ secrets.HOMEBREW_PR_PAT }}
```
</details>

The following sections are not strictly in the same order as the workflow but intended to bucket the different responsibilities.

### Site Manual

A git commit is created in the `cli.github.com` site repository containing the contents of the CLI Manual uploaded by the [`linux`](#linux) job. This is not pushed until the package repository artifacts are set up later.

### Site Package Repositories

The `cli.github.com` website hosts RPM and Debian package repositories to support the [official sources installation instructions](https://github.com/cli/cli/blob/trunk/docs/install_linux.md#official-sources). In order to provide a secure installation method, artifacts in these repositories are signed by a GPG key, which must be loaded into `gpg` for use in later steps. Comments have been added to provide clarity to the script:

```sh
# Import the public and private keys into gpg non-interactively
base64 -d <<<"$GPG_PUBKEY" | gpg --import --no-tty --batch --yes
base64 -d <<<"$GPG_KEY" | gpg --import --no-tty --batch --yes
# Configure gpg so that passphrases can be preset, so that they don't
# have to be provided on every future operation.
echo "allow-preset-passphrase" > ~/.gnupg/gpg-agent.conf
# Inform gpg that it should reload the configuration to apply the previous step
gpg-connect-agent RELOADAGENT /bye
# Store the passphrase for a specific key (referenced by keygrip) in memory.
/usr/lib/gnupg2/gpg-preset-passphrase --preset "$GPG_KEYGRIP" <<<"$GPG_PASSPHRASE"
```

#### RPM

The `.rpm` files uploaded by the [`linux`](#linux) job are signed using [`rpmsign`](https://man7.org/linux/man-pages/man8/rpmsign.8.html). The [`createrepo`](https://linux.die.net/man/8/createrepo) tool is used to generate a `repomd.xml` metadata file which describes the contents of a Red Hat repository. The artifacts and `repomd.xml` file are then copied into the site repository, and the `repomd.xml` is signed using `gpg --yes --detach-sign --armor repodata/repomd.xml`, producing a signature file. Since there is only one private key imported into `gpg`, that key is used for the signing.

> [!WARNING]
> The `createrepo` tool is executed inside a [Docker container](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/script/createrepo.sh) for [package management reasons](https://github.com/cli/cli/pull/2856) that may no longer be true.

#### Debian

The `.deb` files uploaded by the [`linux`](#linux) job are iterated per Debian release (see warning below), using [`reprepro`](https://manpages.debian.org/bookworm/reprepro/reprepro.1.en.html) which produces a directory and file structure for a Debian package repository. The [`./script/distributions`](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/script/distributions) `SignWith` lines indicate the GPG Key ID that `reprepro` should use to sign packages in the created repository. The generated directories are then copied to the site repository.

> [!WARNING]
> There is a note that we should remove [legacy distributions from our list](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.github/workflows/deployment.yml#L329-L332) but no indication of when that would happen.

### Attest Artifacts

[Attestations](https://docs.github.com/en/actions/security-for-github-actions/using-artifact-attestations/using-artifact-attestations-to-establish-provenance-for-builds) are created for each of the release artifacts. For an example see: https://github.com/cli/cli/attestations/4920729

### Publish Release

After all release artifacts have been created, and signed, there are a number of steps taken to make them available to our users.

#### GitHub Release

`gh release create` is invoked to create a new release on GitHub, attaching all the archives, packages and installers, plus a checksum file to allow `gh` users to validate the attached artifacts. The artifact file names are provided to `gh release create` along with a label, as per the command `--help`:

```
Upload a release asset with a display label
$ gh release create v1.2.3 '/path/to/asset.zip#My display label'
```

> [!NOTE]
> It's unclear why human readable display labels were used, beyond comments that it was intentional
> https://github.com/cli/cli/pull/7324
> https://github.com/cli/cli/issues/7470#issuecomment-1556986607

#### Site

In previous steps, a git commit was made for the manual, and files had moved into place for the RPM and Debian package repositories. The package repository structure is committed and pushed, which kicks off a deployment workflow in site repository.

Occasionally, the repository can become unwieldy due to hosting so many large binary artifacts. Instructions can be found in the README for that repository.

#### Homebrew Formula

Using [`mislav/bump-homebrew-formula-action`](https://github.com/mislav/bump-homebrew-formula-action), a PR for the `gh` [`homebrew-core` formula](https://github.com/Homebrew/homebrew-core/blob/master/Formula/g/gh.rb) is created. The fork repository is currently owned by `williammartin` as PRs are [not accepted from organizations.](https://github.com/cli/cli/pull/7953)

`Homebrew/formulae.brew.sh` makes new formula versions available every 15 minutes through scheduled CI workflow. For more information, see https://docs.brew.sh/Formula-Cookbook#an-introduction

## <a id="deepest-dive">Deepest Dive</a>

### <a id="how-script-release-works">How script/release works</a>

[`./script/release`](https://github.com/cli/cli/blob/817eeb26e567de11007c8a82c25e61c7e20e4337/script/release) is used by `gh` maintainers to [create a new release](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/docs/releasing.md). When invoked it executes `gh workflow run` in order to kick off the workflow described in detail above. However, that workflow also calls back into `./script/release` with the `--local` flag resulting in release artifacts being created on the machine invoking it. Each OS specific job in the workflow additionally provides the `--platform` flag.

The surprising behaviour in `./script/release` is that it uses `sed` to modify the base [`.goreleaser.yml` ](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.goreleaser.yml) file, so that only platform specific sections are retained. For example, in the case of of `linux` only the [`linux` build](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.goreleaser.yml#L27) and [`npmfs`](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.goreleaser.yml#L78) section would be configured for `GoReleaser`. The `archive` sections are addressed by [requirements](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.goreleaser.yml#L52) on previous platform builds.

Each build entry in [`.goreleaser.yml` ](https://github.com/cli/cli/blob/756f4ec04abdc9fdbab3fef35b182c546ef1dd17/.goreleaser.yml) specifies the platforms that are supported, for example:

```yml
  - id: linux #build:linux
    goos: [linux]
    goarch: [386, arm, amd64, arm64]
```

### <a id="how-script-pkgmacos-works">How script/pkgmacos works</a>

[./script/pkgmacos](https://github.com/cli/cli/blob/817eeb26e567de11007c8a82c25e61c7e20e4337/script/pkgmacos) is used by the [macos job](#macos) to create a `.pkg` installer. It uses three main utilities:
 * [`lipo`](https://developer.apple.com/documentation/apple-silicon/building-a-universal-macos-binary) to combine the `arm64` and `amd64` binaries into one
 *  [`pkgbuild`](https://www.unix.com/man_page/osx/1/pkgbuild/) to build a "component package", which is the payload to be installed by a MacOS installer. For `gh`, this is `com.github.cli.pkg`. The contents of this package is the universal binary, zsh completions and man pages.
 * [`productbuild`](https://www.unix.com/man_page/osx/1/productbuild/) creates a "product archive" which is used by the MacOS installer. In addition to the "component package", a product archive can contain customized installation elements. For `gh`, we include a `LICENSE` file. We include a [`distribution.xml`](https://github.com/cli/cli/blob/trunk/build/macOS/distribution.xml) file in our repo. which` productbuild` uses.

A good explanation of the difference between `pkgbuild` and `productbuild` can be found on [this Stackoverflow answer](https://stackoverflow.com/questions/74422992/what-is-the-difference-between-pkgbuild-vs-productbuild).
