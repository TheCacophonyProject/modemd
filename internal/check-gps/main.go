package checkgps

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/TheCacophonyProject/go-utils/logging"
	"github.com/alexflint/go-arg"
	"github.com/tarm/serial"
)

var log = logging.NewLogger("info")

type gpsData struct {
	latitude    float64
	longitude   float64
	utcDateTime time.Time
	altitude    float64
	speed       float64
	course      float64
}

type Args struct {
	logging.LogArgs
}

func (Args) Version() string {
	return version
}

var defaultArgs = Args{}
var version = "<not set>"

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

	log.Println("Checking if GPS is enabled")
	out, err := runATCommand("AT+CGPS?")
	if err != nil {
		log.Fatal(err)
	}
	if out != "+CGPS: 1,1" {
		log.Println("Enabling GPS, GPS state was ", out)
		_, err := runATCommand("AT+CGPS=1")
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Println("GPS already enabled")
	}

	//Loop checking for GPS connection.
	for {
		out, err := runATCommand("AT+CGPSINFO")
		if err != nil {
			log.Fatal(err)
		}
		log.Println(out)
		//out = "+CGPSINFO: 4333.256890,S,17237.550876,E,100823,033054.0,10.2,0.0,"
		out = strings.TrimSpace(out)
		out = strings.TrimPrefix(out, "+CGPSINFO:")
		out = strings.TrimSpace(out)
		parts := strings.Split(out, ",")
		gps, err := processGPSOut(parts)
		if err != nil {
			log.Println(err)
		}
		log.Printf("%+v", gps)

		//for _, part := range parts {
		//		log.Println("part:", part)
		//	}
		time.Sleep(5 * time.Second)
	}
}

//+CGPSINFO: 4333.256890,S,17237.550876,E,100823,033054.0,10.2,0.0,

func processGPSOut(parts []string) (*gpsData, error) {
	if len(parts) < 8 {
		return nil, fmt.Errorf("invalid GPS format")
	}
	latRaw := parts[0]
	latNSRaw := parts[1]
	longRaw := parts[2]
	longEWRaw := parts[3]
	utcDateRaw := parts[4]
	utcTimeRaw := parts[5]
	altitudeRaw := parts[6]
	speedRaw := parts[7]
	courseRaw := parts[8]
	//log.Println("latRaw:", latRaw)
	//log.Println("latNSRaw:", latNSRaw)
	//log.Println("longRaw:", longRaw)
	//log.Println("longEW:", longEWRaw)
	//log.Println("utcDateRaw:", utcDateRaw)
	//log.Println("utcTimeRaw:", utcTimeRaw)
	//log.Println("altitude:", altitudeRaw)
	//log.Println("speed:", speedRaw)
	//log.Println("course:", courseRaw)

	if latRaw == "" {
		return nil, nil
	}
	if string(latRaw[4]) != "." {
		return nil, fmt.Errorf("invalid latitude")
	}
	latDeg, err := strconv.ParseFloat(latRaw[:2], 64)
	if err != nil {
		return nil, err
	}
	latMinute, err := strconv.ParseFloat(latRaw[2:], 64)
	if err != nil {
		return nil, err
	}
	latDeg += latMinute / 60
	if latNSRaw == "S" {
		latDeg *= -1
	} else if latNSRaw != "N" {
		return nil, fmt.Errorf("invalid latitude direction")
	}
	log.Println("latDeg:", latDeg)

	if string(longRaw[5]) != "." {
		return nil, fmt.Errorf("invalid longitude")
	}
	longDeg, err := strconv.ParseFloat(longRaw[:3], 64)
	if err != nil {
		return nil, err
	}
	longMinute, err := strconv.ParseFloat(longRaw[3:], 64)
	if err != nil {
		return nil, err
	}
	longDeg += longMinute / 60
	if longEWRaw == "W" {
		latDeg *= -1
	} else if longEWRaw != "E" {
		return nil, fmt.Errorf("invalid longitude direction")
	}
	log.Println("longDeg:", longDeg)

	const layout = "020106-150405.0" // format DDMMYY-hhmmss.s
	dateTime, err := time.Parse(layout, utcDateRaw+"-"+utcTimeRaw)
	log.Println(dateTime.Local().Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, err
	}

	altitude, err := strconv.ParseFloat(altitudeRaw, 64)
	if err != nil {
		return nil, err
	}
	log.Println(altitude)

	speed, err := strconv.ParseFloat(speedRaw, 64)
	if err != nil {
		return nil, err
	}
	log.Println(speed)

	var course float64
	if courseRaw != "" {
		course, err = strconv.ParseFloat(courseRaw, 64)
		if err != nil {
			return nil, err
		}
	}
	log.Println(course)

	return &gpsData{
		latitude:    latDeg,
		longitude:   longDeg,
		utcDateTime: dateTime,
		altitude:    altitude,
		speed:       speed,
		course:      course,
	}, nil
}

func runATCommand(atCommand string) (string, error) {
	log.Printf("Running '%s'", atCommand)

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
		//TODO Add timeout check
		line, err := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if err != nil {
			log.Fatal(err)
		}
		if line == "ERROR" {
			failed = true
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

// ConvertLong converts a longitude in format dddmm.mmmmmm to degrees.
func ConvertLong(long string) (float64, error) {
	if len(long) < 7 {
		return 0, fmt.Errorf("invalid longitude format")
	}

	// Split the longitude string into degrees and minutes.
	degreesPart := long[:3]
	minutesPart := long[3:]

	degrees, err := strconv.ParseFloat(degreesPart, 64)
	if err != nil {
		return 0, err
	}

	minutes, err := strconv.ParseFloat(minutesPart, 64)
	if err != nil {
		return 0, err
	}

	// Convert minutes to degrees.
	minutesInDegrees := minutes / 60.0

	return degrees + minutesInDegrees, nil
}

// ConvertLat converts a latitude in format ddmm.mmmmmm to degrees.
func ConvertLat(lat string) (float64, error) {
	if len(lat) < 6 {
		return 0, fmt.Errorf("invalid latitude format")
	}

	// Split the latitude string into degrees and minutes.
	degreesPart := lat[:2]
	minutesPart := lat[2:]

	degrees, err := strconv.ParseFloat(degreesPart, 64)
	if err != nil {
		return 0, err
	}

	minutes, err := strconv.ParseFloat(minutesPart, 64)
	if err != nil {
		return 0, err
	}

	// Convert minutes to degrees.
	minutesInDegrees := minutes / 60.0

	return degrees + minutesInDegrees, nil
}

/*
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
*/
