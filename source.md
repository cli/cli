# Installation from source

0. Verify that you have Go 1.13+ installed
```
$ go version
go version go1.13.7
```

1. Clone cli into `~/.githubcli`
```
$ git clone https://github.com/cli/cli.git ~/.githubcli
```

2. Compile
```
$ cd ~/.githubcli && make
```

3. Add `~/.githubcli/bin` to your $PATH for access to the gh command-line utility.

  * For **bash**:
  ~~~ bash
  $ echo 'export PATH="$HOME/.githubcli/bin:$PATH"' >> ~/.bash_profile
  ~~~
  
  * For **Zsh**:
  ~~~ zsh
  $ echo 'export PATH="$HOME/.githubcli/bin:$PATH"' >> ~/.zshrc
  ~~~
  
  * For **Fish shell**:
  ~~~ fish
  $ set -Ux fish_user_paths $HOME/.githubcli/bin $fish_user_paths
  ~~~

4. Restart your shell so that PATH changes take effect.

