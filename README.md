- This project uses duplicut as submodule for de-duplicating wordlists

- On first run `git submodule init && git submodule update`
OR
- With future uses run `git submodule update`

- Then run `cd duplicate && make && cd ..`
- Ensure Go is installed `sudo apt install -y golang`
    - Add these to shell rc file (usually .zshrc or .bashrc, echo $SHELL to find out)
        ```
        export GOROOT=/usr/lib/go
        export GOPATH=$HOME/go
        export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
        ```
    - Reload rc file `source ~/.zshrc` (zsh example)

- Install Go packages with `go get ./...`
