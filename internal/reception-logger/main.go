package receptionlogger

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/TheCacophonyProject/go-utils/logging"
	"github.com/alexflint/go-arg"
	"github.com/tarm/serial"
)

var log = logging.NewLogger("info")
var version = "<not set>"

type Args struct {
	logging.LogArgs
}

func (Args) Version() string {
	return version
}

var defaultArgs = Args{}

func procArgs(input []string) (Args, error) {
	args := defaultArgs

	parser, err := arg.NewParser(arg.Config{}, &args)
	if err != nil {
		return Args{}, err
	}
	err = parser.Parse(input)
	if errors.Is(err, arg.ErrHelp) {
		parser.WriteHelp(os.Stdout)
		os.Exit(0)
	}
	if errors.Is(err, arg.ErrVersion) {
		fmt.Println(version)
		os.Exit(0)
	}
	return args, err
}

func Run(inputArgs []string, ver string) error {
	version = ver
	args, err := procArgs(inputArgs)
	if err != nil {
		return fmt.Errorf("failed to parse args: %v", err)
	}
	log = logging.NewLogger(args.LogLevel)

	log.Infof("Running version: %s", version)
	for {
		_, err := runATCommand("AT")
		if err != nil {
			log.Fatal(err)
		}
		reception := readReception()
		band, _ := readBand()
		t := time.Now().Format("2006-01-02 15:04:05")
		out := fmt.Sprintf("%s, %s, %s", t, reception, band)
		log.Println(out)
		appendToFile("/home/pi/reception", out)
		time.Sleep(5 * time.Second)
	}
}

func readReception() string {
	out, err := runATCommand("AT+CSQ")
	if err != nil {
		log.Fatal(err)
	}
	//log.Println(string(out))
	out = strings.TrimPrefix(out, "+CSQ:")
	out = strings.TrimSpace(out)

	parts := strings.Split(out, ",")
	if len(parts) > 1 {
		return parts[0]
	} else {
		log.Fatal(fmt.Errorf("unable to read reception, '%s'", out))
	}
	return out
}

func readBand() (string, error) {
	out, err := runATCommand("AT+CPSI?")
	if err != nil {
		return "", err
	}
	//log.Println(string(out))
	parts := strings.Split(out, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "BAND") {
			return part, nil
		}
	}

	return "", err
}

func runATCommand(atCommand string) (string, error) {
	//log.Printf("Running '%s'", atCommand)

	c := &serial.Config{Name: "/dev/UsbModemAT", Baud: 115200, ReadTimeout: 2 * time.Second}
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	_, err = s.Write([]byte(atCommand + "\r"))
	if err != nil {
		log.Fatal(err)
	}

	// Read and log the response with timestamp
	reader := bufio.NewReader(s)
	failed := false
	for {
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if err != nil {
			log.Fatal(err)
		}
		if line == "ERROR" {
			failed = true
			break
		}
		if line == "OK" {
			break
		}
		if strings.HasPrefix(line, "+") {
			return line, nil
		}
	}
	if failed {
		return "", fmt.Errorf("AT command failed")
	}
	return "", nil
}

func appendToFile(filename string, data string) error {
	// Open the file in append mode or create if it doesn't exist
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Write the data to the file with a new line
	_, err = file.WriteString(data + "\n")
	return err
}
