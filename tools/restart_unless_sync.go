package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/sendgrid/sendgrid-go"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func init() {
	// Make sure the seed is initialized differently each run
	rand.Seed(time.Now().UTC().UnixNano())
}

func main() {
	prod := server{Name: "freecinc"}
	staging := server{Name: "freecinc-staging"}

	prod.check()
	staging.check()
}

type server struct {
	Name string
}

type result struct {
	Success    bool
	Timeliness bool
	Output     string
}

func (s server) check() {

	// For some reason this does not work unless channel is buffered
	channel := make(chan result, 1)

	// Start process
	binary := "/usr/local/bin/task"
	args := "sync"

	// Set TASKRC environment variable
	taskrc := fmt.Sprintf("~/.taskrc-%s", s.Name)
	os.Setenv("TASKRC", taskrc)

	log.Println("binary: ", binary)
	log.Println("args: ", args)
	log.Println("TASKRC env var: ", os.Getenv("TASKRC"))

	command := exec.Command(binary, args)

	// in a separate goroutine, wait for completion
	go runAndWriteToChannelWhenDone(command, channel)

	// Allow 10 seconds for completion
	time.Sleep(10 * time.Second)
	log.Println("Done waiting")

	syncResult := <-channel
	// Print stdout and stderr from task
	log.Println("output: ", syncResult.Output)

	if syncResult.Timeliness == false {
		log.Println("ERROR: Timeliness. Killing process.")
		command.Process.Kill()

		subject := fmt.Sprintf("Sync Failure on %s.com (Timeliness == false)", s.Name)
		sendEmail(subject, "Your server has been restarted")
		s.Restart()
	} else if syncResult.Success == false {
		subject := fmt.Sprintf("Sync Failure on %s.com (Success == false)", s.Name)
		sendEmail(subject, "Your server has been restarted")
		s.Restart()
	} else {
		log.Println("Sync completed with SUCCESS and TIMELINESS")
	}
}

func runAndWriteToChannelWhenDone(command *exec.Cmd, channel chan result) {

	// Write to channel with falses
	channel <- result{Success: false, Timeliness: false}

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	// Start command
	startErr := command.Start()

	if startErr != nil {
		failLoudly("Error starting command: " + startErr.Error())
	}

	command.Wait()

	success := command.ProcessState.Success()

	// Remove original value from channel
	<-channel

	// Replace it with results
	output := stderr.String() + stdout.String()
	channel <- result{Success: success, Timeliness: true, Output: output}

	if success {
		log.Println("SUCCESS")
	} else {
		log.Println("FAILURE")
	}
}

func (s server) Restart() {
	msg := fmt.Sprintf("Restarting server on %s.com", s.Name)
	log.Println(msg)
	uri := fmt.Sprintf("https://%s.com/restart/jeremy", s.Name)
	insecureHttpGet(uri)
}

func insecureHttpGet(uri string) {
	// get uri, but ignore ssl errors

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: transport}

	http.Get(uri)

	_, err := client.Get(uri)
	if err != nil {
		failLoudly(err.Error())
	}
}

func failLoudly(subject string) {
	log.Println(subject)
	sendEmail(subject, "twiddling thumbs")
	panic(subject)
}

func sendEmail(subject string, body string) {
	log.Println("Sending email with subject '" + subject + "' and body '" + body + "'")
	sg := sendgrid.NewSendGridClient("golang", "golang")
	message := sendgrid.NewMail()
	message.AddTo("jworky@gmail.com")

	// Add a random number to the subject so they show up in different threads
	randNumber := rand.Intn(100)
	randNumberStr := strconv.Itoa(randNumber)
	message.SetSubject(subject + randNumberStr)
	message.SetText(body)
	message.SetFrom("jack@sendgrid.com")
	if r := sg.Send(message); r == nil {
		log.Println("Email sent!")
	} else {
		log.Println("ERROR SENDING EMAIL: ", r)
	}
}
