#!/bin/sh
fail_if_err() {
	echo "checking $1: $3"
	eval $2
	if [ "$?" -ne 0 ]; then
		echo "FAILED " $1
		exit 1
	fi
}

fail_if_err "FORMAT" "[ -z $(goimports -l .) ]" "goimports -l ."
fail_if_err "TEST" "go test ./... > /dev/null" "go test ./..."
fail_if_err "VET" "go vet ./..." "go vet ./..."
# go install golang.org/x/lint/golint@latest
fail_if_err "LINT" "golint -set_exit_status \$(go list ./...)" "golint -set_exit_status \$(go list ./...)"
# complexity check
# go install github.com/fzipp/gocyclo/cmd/gocyclo@latest
fail_if_err "CYCLO" "gocyclo -over 30 ." "gocyclo -over 30 ."

# static check
# honnef.co/go/tools/cmd/staticcheck
# from Fyne Conf 2022 https://youtu.be/J8960TmU2jY
