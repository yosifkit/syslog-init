package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	//"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
	//	"golang.org/x/sys/unix"
)

// PR_SET_CHILD_SUBREAPER is defined in <sys/prctl.h> for linux >= 3.4
//const PR_SET_CHILD_SUBREAPER = 36
/*
func registerSubreaper() {
	if os.Getpid() != 1 {
		// try to register as a subreaper
		err := unix.Prctl(PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
		log.Printf("subreaper: %v\n", err == nil)
	}
}
*/

func createSignalListener(signals ...os.Signal) <-chan os.Signal {
	// If no signals are provided, all incoming signals will be relayed to chanel

	// https://golang.org/pkg/os/signal/#Notify
	// Package signal will not block sending to c: the caller must ensure that c has sufficient buffer space to keep up with the expected signal rate. For a channel used for notification of just one signal value, a buffer of size 1 is sufficient.
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal is sent.
	incomingSigs := make(chan os.Signal, 1024)
	signal.Notify(incomingSigs, signals...)

	return incomingSigs
}

func zombieReaper(incomingChildSigs <-chan os.Signal) {
	for {
		<-incomingChildSigs
		// https://github.com/docker/docker/issues/11529
		var (
			status syscall.WaitStatus
			usage  syscall.Rusage
		)
		for {
			pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, &usage)
			if err != nil || pid == 0 {
				break
			}
		}
	}
}

func forwardSignals(proc *os.Process, incomingSigs <-chan os.Signal) {
	for {
		sig := <-incomingSigs
		err := proc.Signal(sig)
		if err != nil {
			// more here? nothing?
			log.Println("Unable to forward signal:", sig)
		}
	}
}

func syslogRun(scanner *bufio.Scanner) {
	for scanner.Scan() {
		line := scanner.Text()
		// TODO remove syslog extra data?
		fmt.Printf("%s\n", line)
	}
	if err := scanner.Err(); err != nil {
		log.Println("scanner error:", err)
	}
}

func syslogCreate(socket string) *bufio.Scanner {
	unixAddr, err := net.ResolveUnixAddr("unixgram", socket)
	if err != nil {
		log.Fatalln("unable to resolve:", err)
	}

	listener, err := net.ListenUnixgram("unixgram", unixAddr)
	if err != nil {
		if syslogDebug != "" {
			log.Fatalln("listen error:", err)
		} else {
			// do nothing??
		}
	}

	scanner := bufio.NewScanner(listener)
	return scanner
}

var (
	syslogDebug = os.Getenv("SYSLOG_DEBUG")
)

func debugLog(format string, v ...interface{}) {
	if syslogDebug != "" {
		log.Printf(("syslog_init: " + format + "\n"), v...)
	}
}

func main() {
	// get config from the environment
	syslogSocket := os.Getenv("SYSLOG_SOCKET")
	//subReaper := os.Getenv("INIT_SUBREAPER")

	// TODO flags to control whether syslog is wanted
	if len(os.Args) < 1 {
		log.Fatalln("Error: no command specified")
	}

	// check path for the executable
	filename := os.Args[1]
	filepath, err := exec.LookPath(filename)
	if err != nil {
		log.Fatalf("Unable to find [%s] in PATH\n", filename)
	}
	// we don't rely on exec.Command since it only checks LookPath error
	// when you Run or Start it and we already checked it \o/
	cmdToRun := &exec.Cmd{
		Path: filepath,
		Args: os.Args[1:],
	}
	// connect all the pipes http://stackoverflow.com/a/14885714
	cmdToRun.Stdout = os.Stdout
	cmdToRun.Stderr = os.Stderr
	cmdToRun.Stdin = os.Stdin
	signal.Ignore(syscall.SIGTTIN, syscall.SIGTTOU)
	// setup process to have its own process group
	cmdToRun.SysProcAttr = &syscall.SysProcAttr{
		Foreground: terminal.IsTerminal(0),
		Setpgid:    true,
		Pgid:       0,
	}

	// setup a listener for SIGCHILD
	childSignals := createSignalListener(syscall.SIGCHLD)
	// start the (grim)reaper
	// and reap all the children
	go zombieReaper(childSignals)

	// setup a listener for all the signals we want to forward

	incomingSignalsToForward := createSignalListener(
		syscall.SIGTERM,
		syscall.SIGINT,
		syscall.SIGHUP,
		syscall.SIGUSR1,
		syscall.SIGUSR2,
	)

	if syslogSocket != "" {
		// handle syslogSocket file existing (ie, we are restarted)
		if err := os.Remove(syslogSocket); err != nil && !os.IsNotExist(err) {
			log.Fatalln(err)
		}

		syslog := syslogCreate(syslogSocket)
		go syslogRun(syslog)
	}

	err = cmdToRun.Start()
	if err != nil {
		log.Fatalln(err)
	}
	go forwardSignals(cmdToRun.Process, incomingSignalsToForward)

	err = cmdToRun.Wait()
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			waitStatus := exitError.Sys().(syscall.WaitStatus)
			os.Exit(waitStatus.ExitStatus())
		}
		// TODO can't wait since our reaper already did
		fmt.Printf("Unable to wait on cmd. %v\n", err)
		os.Exit(1)
	}
}
