## Install for macOS
### Homebrew
To install:
```sh
brew install github/gh/gh
```
To upgrade:
```sh
brew upgrade gh
```

### Manual install
1. Download the `*_macOS_amd64.tar.gz` file from the [releases page](https://github.com/github/homebrew-gh/releases/latest)
2. `tar -xf gh_*_macOS_amd64.tar.gz`
3. Copy the uncompressed `gh` somewhere to your PATH (e.g. `cp gh_*_macOS_amd64/bin/gh /usr/local/bin/`)

## Install for Windows
1. Download the `*.msi` installer from the [releases page](https://github.com/github/homebrew-gh/releases/latest)
2. Run the installer

### Uninstall from Windows
1. Search for "remove programs" in the start menu
2. Choose “Add or remove programs”
3. Find “GitHub CLI” on the list
4. Click on it and choose “Uninstall”

## Install for Linux
### Debian/Ubuntu Linux

1. `sudo apt install git` if you don't already have git
2. Download the `.deb` file from the [releases page](https://github.com/github/homebrew-gh/releases/latest)
3. `sudo dpkg -i gh_*_linux_amd64.deb`  install the downloaded file

_(Uninstall with `sudo apt remove gh`)_

### Fedora/Centos Linux

1. Download the `.rpm` file from the [releases page](https://github.com/github/homebrew-gh/releases/latest)
2. `sudo yum localinstall gh_*_linux_amd64.rpm` install the downloaded file

_(Uninstall with `sudo yum remove gh`)_

### Other Linux

1. Download the `*_linux_amd64.tar.gz` file from the [releases page](https://github.com/github/homebrew-gh/releases/latest)
2. `tar -xf gh_*_linux_amd64.tar.gz`
3. Copy the uncompressed `gh` somewhere to your PATH (e.g. `sudo cp gh_*_linux_amd64/bin/gh /usr/local/bin/`)


_(Uninstall with `rm`)_
