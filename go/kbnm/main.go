package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/qrtz/nativemessaging"
)

// Version is the build version of kbnm, overwritten during build.
const Version = "dev"

// Response from the kbnm service
type Response struct {
	Client  int         `json:"client"`
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Result  interface{} `json:"result",omitempty`
}

// Request to the kbnm service
type Request struct {
	Client int    `json:"client"`
	Method string `json:"method"`
	To     string `json:"to"`
	Body   string `json:"body"`
}

var plainFlag = flag.Bool("plain", false, "newline-delimited JSON IO, no length prefix")
var versionFlag = flag.Bool("version", false, "print the version and exit")

func main() {
	flag.Parse()

	h := Handler()

	// Native messages include a prefix which describes the length of each message.
	var in nativemessaging.JSONDecoder
	var out nativemessaging.JSONEncoder

	if *plainFlag {
		// Used for testing interactively
		in = json.NewDecoder(os.Stdin)
		out = json.NewEncoder(os.Stdout)
	} else {
		// Used as part of the NativeMessaging API
		in = nativemessaging.NewNativeJSONDecoder(os.Stdin)
		out = nativemessaging.NewNativeJSONEncoder(os.Stdout)
	}

	abort := false
	for {
		var resp Response
		var req Request

		err := in.Decode(&req)

		if err != nil {
			// Input failed to parse; we can't guarantee future inputs will
			// get into a parseable state, so we abort after sending an error
			// response.
			abort = true
		} else {
			resp.Result, err = h.Handle(&req)
		}

		if err == io.EOF {
			// Closed
			break
		} else if err != nil {
			resp.Status = "error"
			resp.Message = err.Error()
		} else {
			// Success
			resp.Status = "ok"
		}
		resp.Client = req.Client

		err = out.Encode(resp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s", err)
			os.Exit(1)
		}

		if abort {
			os.Exit(2)
		}
	}
}
