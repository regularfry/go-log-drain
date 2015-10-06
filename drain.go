package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"io/ioutil"
)


/* Global handle to the logfile.  When we receive a SIGHUP, we close and reopen. */
var out *os.File
var outFilename string
var ops chan Op

type Op interface {
	Call() error
}

// Our operations are going to be:
// 1. Handle an upload
// 2. Reopen our output file
// 3. Quit.

type UploadOp struct {
	body []byte
}
type ReopenOp struct {}
type QuitOp struct {}


// Handling an upload is appending it to our current log file, with a
// newline for separation.  These all happen on the same goroutine, so
// the writes can't get interleaved.
func (upload UploadOp) Call() error {
	_, err := out.Write(upload.body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}
	_,err = out.Write([]byte("\n"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}
	return nil;
}

// A reopen is a close followed by the same Open call as we did at
// startup
func (op ReopenOp) Call() error {
	return reopen()
}


// Quitting just bombs out.  Again, because this is handled on the
// same goroutine as the upload, this won't interrupt a log write.
func (quit QuitOp) Call() error{
	os.Exit(0)
	return nil
}

// Open our destination file and stash it in our global var.
func open() error{
	var err error
	out,err = os.OpenFile(outFilename,os.O_WRONLY|os.O_CREATE|os.O_APPEND,os.FileMode(0666));
	if err != nil{
		fmt.Fprintf(os.Stderr, "Couldn't open %s: %s\n", outFilename, err)
		os.Exit(1);
	}
	return nil
}

// Close our global `out` file, then open it again.
func reopen() error{
	err := out.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Couldn't close %s: %s\n", outFilename, err)
		os.Exit(1)
	}
	return open()
}

// Set up a chan to receive OS signals, and only listen for HUP.  When we get one, convert it to a ReopenOp
func listenForSignals(){
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP)
	/* No facility for shutting this goroutine down, not sure I care */
	go func(){
		for {
			<- signalChan
			ops <- ReopenOp{}
		}
	}()
}

func postHandler(w http.ResponseWriter, r *http.Request){
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Body read failed: %s\n", err)
	}
	ops <- UploadOp{body: body}
}

func listenForWeb(){
	http.HandleFunc("/", postHandler)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
	}
}

func opLoop(){
	for {
		op := <- ops
		op.Call()
	}
}

func main(){
	outFilename = os.Args[1]
	open()
	ops = make(chan Op,3)
	go opLoop()
	// Spin off a goroutine to listen for SIGHUP and push the resulting op to ops
	listenForSignals()
	// Listen for web posts.  This blocks.
	listenForWeb()
}
