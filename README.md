# yarn

[![Go Reference](https://pkg.go.dev/badge/github.com/DrJosh9000/yarn.svg)](https://pkg.go.dev/github.com/DrJosh9000/yarn)
[![Go Report Card](https://goreportcard.com/badge/github.com/DrJosh9000/yarn)](https://goreportcard.com/report/github.com/DrJosh9000/yarn)

The yarn package is a work-in-progress Go implementation of the
[YarnSpinner](https://github.com/YarnSpinnerTool/YarnSpinner) virtual machine
and dialogue system. Given a compiled `.yarn` file (into the VM bytecode and
string table) and `DialogueHandler` implementation, the `yarn.VM` can execute
the program as the original YarnSpinner VM would, delivering lines, options, and
commands to the handler.