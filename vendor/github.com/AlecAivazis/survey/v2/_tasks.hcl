task "install-deps" {
    description = "Install all of package dependencies"
    pipeline = [
        "go get {{.files}}",
    ]
}

task "tests" {
    description = "Run the test suite"
    command = "go test {{.files}}"
    environment {
        GOFLAGS = "-mod=vendor"
    }
}

variables {
    files = "$(go list -v ./... | grep -iEv \"tests|examples\")"
}

