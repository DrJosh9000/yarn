# yarn

[![Go Reference](https://pkg.go.dev/badge/github.com/DrJosh9000/yarn.svg)](https://pkg.go.dev/github.com/DrJosh9000/yarn)
[![Go Report Card](https://goreportcard.com/badge/github.com/DrJosh9000/yarn)](https://goreportcard.com/report/github.com/DrJosh9000/yarn)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/DrJosh9000/yarn/blob/main/LICENSE)

The yarn package is a work-in-progress Go implementation of the
[YarnSpinner](https://github.com/YarnSpinnerTool/YarnSpinner) virtual machine
and dialogue system. Given a compiled `.yarn` file (into the VM bytecode and
string table) and `DialogueHandler` implementation, the `yarn.VirtualMachine`
can execute the program as the original YarnSpinner VM would, delivering lines,
options, and commands to the handler.

## Usage

1. Compile your `.yarn` file, e.g. with the
   [YarnSpinner Console](https://github.com/YarnSpinnerTool/YarnSpinner-Console):

   ```shell
   ysc compile Example.yarn
   ```

   This produces two files: the VM bytecode `.yarnc`, and a string table
   `.csv`.

2. Implement a `DialogueHandler`, which receives events from the VM. Here's an
   example that plays the dialogue on the terminal:

   ```go
   type MyHandler struct{
       stringTable yarn.StringTable
       // ... and your own fields ...
   }

   func (m *MyHandler) Line(line yarn.Line) error {
       // line.ID is the key into the string table.
       // StringTableRow.Text is the "string".
       fmt.Println(m.stringTable[line.ID].Text)
       // You can block in here to give the player time to read the text.
       fmt.Println("\n\nPress ENTER to continue")
       fmt.Scanln()
       return nil
   }

   func (m *MyHandler) Options(opts []yarn.Option) (int, error) {
       fmt.Println("Choose:")
       for _, opt := range opts {
           fmt.Printf("%d: %s\n", opt.ID, m.stringTable[opt.Line.ID].Text)
       }
       fmt.Print("Enter the number of your choice: ")
       var choice int
       fmt.Scanf("%d", &choice)
       return choice, nil
   }

   // ... and also the other methods. 
   ```

   See `cmd/yarnrunner.go` for a complete example.

3. Load the two files, your `DialogueHandler`, and a `VariableStorage` into a
   `VirtualMachine`, and then pass the name of the first node to `Run`:

   ```go
   package main
   
   import (
       "io/ioutil"
       "os"
       "google.golang.org/protobuf/proto"
       "github.com/DrJosh9000/yarn"
       yarnpb "github.com/DrJosh9000/yarn/bytecode"
   )
   
   func main() {
       // Load the files:
       // (error handling omitted for brevity)
       yarnc, _ := ioutil.ReadFile("Example.yarn.yarnc")
       program := new(yarnpb.Program)
       proto.Unmarshal(yarnc, program)
       csv, _ := os.Open("Example.yarn.csv")
       defer csv.Close()
       stringTable, _ := yarn.ReadStringTable(csv)
       // Set up the DialogueHandler and the VirtualMachine:
       myHandler := &MyHandler{
           stringTable: stringTable,
       }
       vm := &yarn.VirtualMachine{
           Program: program,
           Handler: myHandler,
           Vars: make(yarn.MapVariableStorage), 
           // or your own VariableStorage implementation
       }
       // Run the VirtualMachine!
       if err := vm.Run("Start"); err != nil {
           log.Printf("VirtualMachine exeption: %v", err)
       }
   }
   ```

In a more typical game, `vm.Run` would happen in a separate goroutine. To avoid
the VM delivering all the lines and options at once, your `DialogueHandler` is
free to block execution - for example, on a channel operation:

```go
type MyHandler struct {
    stringTable yarn.StringTable

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
    m.dialogueDisplay.Show(m.stringTable[line.ID].Text+"\n\nPress ENTER to continue")
    
    // Go into waiting-for-player-input state
    m.setWaiting(true)

    // Recieve on m.next, which blocks until another goroutine sends on it.
    <-m.next
    return nil
}

// Update is called on every tick by the game engine, which is a separate
// goroutine to the one the VM is running in.
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
