package main

//go:generate C:\!Dev\GOPATH\src\github.com\vsdutka\gover\gover.exe
import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"

	"github.com/kardianos/service"
	_ "golang.org/x/tools/go/ssa"
)

var (
	logger     service.Logger
	loggerLock sync.Mutex
	verFlag    *bool
	svcFlag    *string
	dsnFlag    *string
	certFlag   *string
	keyFlag    *string
	portFlag   *int
	debugFlag  *bool
)

const (
	confServiceDispName = "Dash HTTP Server on Port %v"
	confServiceName     = "DashHTTPServer_%v"
)

func logInfof(format string, a ...interface{}) error {
	loggerLock.Lock()
	defer loggerLock.Unlock()
	if logger != nil {
		return logger.Infof(format, a...)
	}
	return nil
}
func logError(v ...interface{}) error {
	loggerLock.Lock()
	defer loggerLock.Unlock()
	if logger != nil {
		return logger.Error(v)
	}
	return nil
}

// Program structures.
//  Define Start and Stop methods.
type program struct {
	exit chan struct{}
}

func (p *program) Start(s service.Service) error {
	if service.Interactive() {
		logInfof("Service \"%s\" is running in terminal.", serviceDispName())
	} else {
		logInfof("Service \"%s\" is running under service manager.", serviceDispName())
	}
	p.exit = make(chan struct{})

	// Start should not block. Do the actual work async.
	go p.run()
	return nil
}
func (p *program) run() {
	startServer()
	logInfof("Service \"%s\" is started.", serviceDispName())
	for {
		select {
		case <-p.exit:
			return
		}
	}
}
func (p *program) Stop(s service.Service) error {
	// Any work in Stop should be quick, usually a few seconds at most.
	logInfof("Service \"%s\" is stopping.", serviceDispName())
	stopServer()
	logInfof("Service \"%s\" is stopped.", serviceDispName())
	close(p.exit)
	return nil
}

// Service setup.
//   Define service config.
//   Create the service.
//   Setup the logger.
//   Handle service controls (optional).
//   Run the service.
func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	flag.Usage = usage
	verFlag = flag.Bool("version", false, "Show version")
	svcFlag = flag.String("service", "", fmt.Sprintf("Control the system service. Valid actions: %q\n", service.ControlAction))
	dsnFlag = flag.String("dsn", "", "    MS SQL connection string (server=%s;user id=%s;password=%s;port=%d)")
	certFlag = flag.String("cert", "", "    cert file name")
	keyFlag = flag.String("key", "", "    key file name")
	portFlag = flag.Int("port", 9999, "Port")
	debugFlag = flag.Bool("debug", false, "Produce reques/response dumps")

	flag.Parse()

	if *verFlag == true {
		fmt.Println("Version: ", VERSION)
		fmt.Println("Build:   ", BUILD_DATE)
		os.Exit(0)
	}

	if *dsnFlag == "" {
		usage()
		os.Exit(2)
	}

	svcConfig := &service.Config{
		Name:        fmt.Sprintf(confServiceName, *portFlag),
		DisplayName: serviceDispName(),
		Description: serviceDispName(),
		Arguments: []string{
			fmt.Sprintf("-dsn=%s", *dsnFlag),
			fmt.Sprintf("-port=%v", *portFlag),
			fmt.Sprintf("-cert=%v", *certFlag),
			fmt.Sprintf("-key=%v", *keyFlag),
		},
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatal(err)
	}
	errs := make(chan error, 5)
	func() {
		loggerLock.Lock()
		defer loggerLock.Unlock()
		logger, err = s.Logger(errs)
		if err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		for {
			err := <-errs
			if err != nil {
				log.Print(err)
			}
		}
	}()

	if len(*svcFlag) != 0 {
		err := service.Control(s, *svcFlag)
		if err != nil {
			log.Printf("Valid actions: %q\n", service.ControlAction)
			log.Fatal(err)
		}
		return
	}
	err = s.Run()
	if err != nil {
		logError(err)
	}
}

func serviceDispName() string {
	return fmt.Sprintf(confServiceDispName, *portFlag)
}

const usageTemplate = `dash - Dashboard server for OMNI tracker

Usage: dash commands

The commands are:
`

func usage() {
	fmt.Fprintln(os.Stderr, usageTemplate)
	flag.PrintDefaults()
}
