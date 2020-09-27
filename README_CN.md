# GitHub CLI

[English][]

`gh` 是一个命令行工具，是Github的命令行形式。 通过 `gh`，你能在命令行使用pull requests， issues等等Github网页版功能。

![screenshot of gh pr status](https://user-images.githubusercontent.com/98482/84171218-327e7a80-aa40-11ea-8cd1-5177fc2d0e72.png)

## 获取

GitHub CLI 库目前托管在GitHub.com 和GitHub Enterprise Server 2.20+，能被安装到macOS，Windows和Linux。


## 文档

阅读 [official docs][] 获取使用方法及更多信息。



## 期待您的反馈

我们期待您对`gh`的反馈。若您找到了bug或是想看到`gh`增加些新特性，请参考贡献页 [contributing page][]。

<!-- this anchor is linked to from elsewhere, so avoid renaming it -->
## 多平台安装

### macOS

可通过 Homebrew 和 MacPorts 安装 `gh`。

#### Homebrew

|安装:|升级:|
|---|---|
|`brew install gh`|`brew upgrade gh`|

#### MacPorts

|安装:|升级:|
|---|---|
|`sudo port install gh`|`sudo port selfupdate && sudo port upgrade gh`|



### Linux

参见 [Linux installation docs](/docs/install_linux.md)。

### Windows

可通过 [scoop][]，[Chocolatey][] 以及直接下载MSI安装`gh`。

#### scoop

安装:

```powershell
scoop bucket add github-gh https://github.com/cli/scoop-gh.git
scoop install gh
```

升级:

```powershell
scoop update gh
```

#### Chocolatey

|安装:|升级:|
|---|---|
|`choco install gh`|`choco upgrade gh`|


#### 已签发的 MSI

可在 [releases page][] 下载MSI。

### 其他平台

请从 [releases page][] 下载打包好的二进制文件。

### 源码编译

从这里 [build GitHub CLI from source][build from source] 查看如何从源码编译 `gh` 工具。

## 和hub工具的比较

多年以来，[hub][] 一直是Github非官方版的命令行工具。`gh` 则是一个全新的工具，它的设计和hub有着根本的不同。通过 `gh`，大家可以去探索官方版命令行工具到底能够做成什么样。尽管两个工具都将GitHub搬到了命令行下，但hub表现得更像是 `git` 代理，而 `gh`则是一个完全独立的工具。 查看[more detailed explanation][gh-vs-hub]以获取更多信息。


[official docs]: https://cli.github.com/manual
[scoop]: https://scoop.sh
[Chocolatey]: https://chocolatey.org
[releases page]: https://github.com/cli/cli/releases/latest
[hub]: https://github.com/github/hub
[contributing page]: https://github.com/cli/cli/blob/trunk/.github/CONTRIBUTING.md
[gh-vs-hub]: /docs/gh-vs-hub.md
[build from source]: /docs/source.md
[English]: README.md
