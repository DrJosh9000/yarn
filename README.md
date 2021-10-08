# yarn

[![Go Reference](https://pkg.go.dev/badge/github.com/DrJosh9000/yarn.svg)](https://pkg.go.dev/github.com/DrJosh9000/yarn)
[![Go Report Card](https://goreportcard.com/badge/github.com/DrJosh9000/yarn)](https://goreportcard.com/report/github.com/DrJosh9000/yarn)

The yarn package is a work-in-progress Go implementation of the
[YarnSpinner](https://github.com/YarnSpinnerTool/YarnSpinner) virtual machine
and dialogue system. Given a compiled `.yarn` file (into the VM bytecode and
string table) and `DialogueHandler` implementation, the `yarn.VirtualMachine`
can execute the program as the original YarnSpinner VM would, delivering lines,
options, and commands to the handler.

## Usage

1.  Compile your `.yarn` file, e.g. with the 
    [YarnSpinner Console](https://github.com/YarnSpinnerTool/YarnSpinner-Console):
    ```shell
    $ ysc compile Example.yarn
    ```
    This produces two files: the VM bytecode `.yarnc`, and a string table
    `.csv`.

2.  Load the two files:
    ```go
    // error handling omitted for brevity
    import (
        "io/ioutil"
        "os"

        "google.golang.org/protobuf/proto"
        "github.com/DrJosh9000/yarn"
        yarnpb "github.com/DrJosh9000/yarn/bytecode"
    )

    func main() {
        yarnc, _ := ioutil.ReadFile("Example.yarn.yarnc")
        program := new(*yarnpb.Program)
        proto.Unmarshal(yarnc, program)

        csv, _ := os.Open("Example.yarn.csv")
        defer csv.Close()
        stringTable, _ := yarn.ReadStringTable(csv)
        //...
    }
    ```

3.  Implement a `DialogueHandler`, which receives events from the VM:
    ```go
    type MyHandler struct{
        stringTable yarn.StringTable
        vm          *yarn.VirtualMachine
        // or your own type
    }

    func (m *MyHandler) Line(line yarn.Line) error {
        // line.ID is the key into the string table.
        // StringTableRow.Text is the "string".
        fmt.Println(m.stringTable[line.ID].Text)
        return nil
    }

    func (m *MyHandler) Options(opts []yarn.Option) error {
        // demo handler that always picks the first option
        return m.vm.SetSelectedOption(opts[0].ID)
    }

    // .. and the others
    ```

4.  Set up the VM:
    ```go
    myHandler := &MyHandler{
        stringTable: stringTable,
    }
    vm := &yarn.VirtualMachine{
        Program: program,
        Handler: myHandler,
        Vars: make(yarn.MapVariableStorage), 
        // or your own VariableStorage implementation
    }
    myHandler.vm = vm

    vm.SetNode("Start")
    ```

5.  `Continue` the VM - it generally waits for a `Continue` after 
    delivering each line, set of options, and so on:
    ```go
    // very simplified continue loop
    for !complete {
        if err := vm.Continue(); err != nil {
            break
        }
    }
    ```