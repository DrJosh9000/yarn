# yarn

A Go implementation of parts of Yarn Spinner 2.2.

Derived from [github.com/DrJosh9000/yarn](https://github.com/DrJosh9000/yarn), but updated to use most the recent Protobuf files and include more tests from upstream.

![Build status](https://github.com/DrJosh9000/yarn/actions/workflows/go.yml/badge.svg)
[![Go Reference](https://pkg.go.dev/badge/github.com/DrJosh9000/yarn.svg)](https://pkg.go.dev/github.com/DrJosh9000/yarn)
[![Go Report Card](https://goreportcard.com/badge/github.com/DrJosh9000/yarn)](https://goreportcard.com/report/github.com/DrJosh9000/yarn)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/DrJosh9000/yarn/blob/main/LICENSE)

The yarn package is a Go implementation of the
[Yarn Spinner 2.0](https://github.com/YarnSpinnerTool/YarnSpinner) dialogue
system. Given a compiled `.yarn` file (into the VM bytecode and string table)
and `DialogueHandler` implementation, the `VirtualMachine` can execute the
program as the original Yarn Spinner VM would, delivering lines, options, and
commands to the handler.

## Supported features

* ✅ All Yarn Spinner 2.0 machine opcodes, instruction forms, and standard
     functions.
* ✅ Custom functions, similar to the `text/template` package.
* ✅ Yarn Spinner CSV string tables.
* ✅ String substitutions (`Hello, {0} - you're looking well!`).
* ✅ `select` format function (`Hey [select value={0} m="bro" f="sis" nb="doc"]`).
* ✅ `plural` format function (`That'll be [plural value={0} one="% dollar" other="% dollars"]`).
* ✅ `ordinal` format function (`You are currently [ordinal value={0} one="%st" two="%nd" few="%rd" other="%th"] in the queue`).
  * ✅ ...including using Unicode CLDR for cardinal/ordinal form selection
    (`en-AU` not assumed!)
* ✅ Custom markup tags are also parsed, and rendered to an `AttributedString`.
* ✅ `visited` and `visit_count`
* ✅ Built-in functions like `dice`, `round`, and `floor` that are mentioned in the Yarn Spinner documentation.

## Usage

1. Compile your `.yarn` file. You can probably get the compiled output from a  
   Unity project, or you can compile without using Unity with a tool like the
   [Yarn Spinner Console](https://github.com/YarnSpinnerTool/YarnSpinner-Console):

   ```shell
   ysc compile Example.yarn
   ```

   This produces two files: the VM bytecode `.yarnc`, and a string table
   `.csv`.

2. Implement a `DialogueHandler`, which receives events from the VM. Here's an
   example that plays the dialogue on the terminal:

   ```go
   type MyHandler struct{
       stringTable *yarn.StringTable
       // ... and your own fields ...
   }

   func (m *MyHandler) Line(line yarn.Line) error {
       // StringTable's Render turns the Line into a string, applying all the
       // substitutions and format functions that might be present.
       text, _ := m.stringTable.Render(line)
       fmt.Println(text)
       // You can block in here to give the player time to read the text.
       fmt.Println("\n\nPress ENTER to continue")
       fmt.Scanln()
       return nil
   }

   func (m *MyHandler) Options(opts []yarn.Option) (int, error) {
       fmt.Println("Choose:")
       for _, opt := range opts {
           text, _ := m.stringTable.Render(opt.Line)
           fmt.Printf("%d: %s\n", opt.ID, text)
       }
       fmt.Print("Enter the number of your choice: ")
       var choice int
       fmt.Scanln(&choice)
       return choice, nil
   }

   // ... and also the other methods. 
   // Alternatively you can embed yarn.FakeDialogueHandler in your handler.
   ```

3. Load the two files, your `DialogueHandler`, a `VariableStorage`, and any
   custom functions, into a
   `VirtualMachine`, and then pass the name of the first node to `Run`:

   ```go
   package main
   
   import "github.com/DrJosh9000/yarn"
   
   func main() {
       // Load the files (error handling omitted for brevity):
       program, stringTable, _ := yarn.LoadFiles("Example.yarn.yarnc", "Example.yarn.csv", "en-AU")

       // Set up your DialogueHandler and the VirtualMachine:
       myHandler := &MyHandler{
           stringTable: stringTable,
       }
       vm := &yarn.VirtualMachine{
           Program: program,
           Handler: myHandler,
           Vars: make(yarn.MapVariableStorage), // or your own VariableStorage implementation
           FuncMap: yarn.FuncMap{ // this is optional
               "last_value": func(x ...interface{}) interface{} {
                   return x[len(x)-1]
               },
               // or your own custom functions!
           }
       }

       // Run the VirtualMachine starting with the Start node!
       vm.Run("Start")
   }
   ```

See `cmd/yarnrunner.go` for a complete example.

## Usage notes

Note that using an earlier Yarn Spinner compiler will result in some unusual
behaviour when compiling Yarn files with newer features. For example, with v1.0
`<<jump ...>>` may be compiled as a command. Your implementation of `Command`
may implement `jump` by calling the `SetNode` VM method.

If you need the tags for a node, you can read these from the `Node` protobuf
message directly. Source text of a `rawText` node can be looked up manually:

```go
prog, st, _ := yarn.LoadFiles("testdata/Example.yarn.yarnc", "testdata/Example.yarn.csv", "en")
node := prog.Nodes["LearnMore"]
// Tags for the LearnMore node:
fmt.Println(node.Tags)
// Source text string ID:
fmt.Println(node.SourceTextStringID)
// Source text is in the string table:
fmt.Println(st.Table[node.SourceTextStringID].Text)
```

In a typical game, `vm.Run` would happen in a separate goroutine. To avoid the
VM delivering all the lines, options, and commands at once, your
`DialogueHandler` implementation is allowed to block execution of the VM
goroutine - for example, using a channel operation:

```go
type MyHandler struct {
    stringTable *yarn.StringTable

    dialogueDisplay Component

    // next is used to block Line from returning until the player is ready for
    // more tasty, tasty content.
    next chan struct{}

    // waiting tracks whether the game is waiting for player input.
    // It is guarded by a mutex since it is changed by two different
    // goroutines.
    waitingMu sync.Mutex
    waiting   bool
}

func (m *MyHandler) setWaiting(w bool) {
    m.waitingMu.Lock()
    m.waiting = w
    m.waitingMu.Unlock()
}

// Line is called from the goroutine running VirtualMachine.Run.
func (m *MyHandler) Line(line yarn.Line) error {
    text, _ := m.stringTable.Render(line)
    m.dialogueDisplay.Show(text)
    
    // Go into waiting-for-player-input state
    m.setWaiting(true)

    // Recieve on m.next, which blocks until another goroutine sends on it.
    <-m.next
    return nil
}

// Update is called on every tick by the game engine, which is a separate
// goroutine to the one the virtual machine is running in.
func (m *MyHandler) Update() error {
    //...
    if m.waiting && inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
        // Hide the dialogue display.
        m.dialogueDisplay.Hide()
        // No longer waiting for player input.
        m.setWaiting(false)
        // Send on m.next, which unblocks the call to Line.
        // Do this after setting m.waiting to false.
       m.next <- struct{}{}
    }
    //...
}
```

## Licence

This project is available under the Apache 2.0 license. See the `LICENSE` file
for more information.

The `bytecode` and `testdata` directories contains files or derivative works
from Yarn Spinner. See `bytecode/README.md` and `testdata/README.md` for more
information.
