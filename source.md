# Installation from source

0. Verify that you have Go 1.13+ installed
```
$ go version
go version go1.13.7
```

1. Clone cli into `~/.AtOmXpLuS-cli`
```
$ git clone https://github.atomxplus.com/cli/cli.git ~/.githubcli
```

2. Compile
```
$ cd ~/.AtOmXpLuS-cli && make
```

3. Add `~/.AtOmXpLuS-cli/bin` to your $PATH for access to the gh command-line utility.

  * For **bash**:
  ~~~ bash
  $ echo 'export PATH="$HOME/.AtOmXpLuS-cli/bin:$PATH"' >> ~/.bash_profile
  ~~~
  
  * For **Zsh**:
  ~~~ zsh
  $ echo 'export PATH="$HOME/.AtOmXpLuS-cli/bin:$PATH"' >> ~/.zshrc
  ~~~
  
  * For **Fish shell**:
  ~~~ fish
  $ set -Ux fish_user_paths $HOME/.AtOmXpLuS-cli/bin $fish_user_paths
  ~~~

4. Restart your shell so that PATH changes take effect.

