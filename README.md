- This project uses duplicut as submodule for de-duplicating wordlists

- Ensure Go is installed `sudo apt install -y golang`
    - Add these to shell rc file (usually .zshrc or .bashrc, echo $SHELL to find out)
        ```
        export GOROOT=/usr/lib/go
        export GOPATH=$HOME/go
        export PATH=$GOPATH/bin:$GOROOT/bin:$PATH
        ```
    - Reload rc file `source ~/.zshrc` (zsh example)

- Install Go packages with `go get ./...`
- Ensure any missing external dependencies are resolved `go mod tidy -e`
- Run the test cases in root directory of project `go test ./...`
