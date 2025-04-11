package main

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"os"
	"regexp"
	"time"

	"github.com/schollz/progressbar/v3"
)

const (
	VERSION     = "1.2.0"
	SERVER_PORT = 5000
	BUFFER_SIZE = 256
	ACK_MSG     = "ACK"
	CRC_OK_MSG  = "CRC_OK"
)

// Compute CRC32 checksum
func calculateCRC32(filePath string) (uint32, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	hash := crc32.NewIEEE()
	buffer := make([]byte, BUFFER_SIZE)
	for {
		bytesRead, err := file.Read(buffer)
		if bytesRead > 0 {
			hash.Write(buffer[:bytesRead])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}

	return hash.Sum32(), nil
}

func sendFirmware(firmwareFile, serverIP string) {
	// Read firmware file
	firmwareData, err := os.ReadFile(firmwareFile)
	if err != nil {
		fmt.Printf("Error: Firmware file %s not found\n", firmwareFile)
		return
	}

	firmwareSize := len(firmwareData)
	fmt.Printf("Sending firmware file: %s, size: %d bytes\n", firmwareFile, firmwareSize)

	// Compute CRC32
	firmwareCRC32, err := calculateCRC32(firmwareFile)
	if err != nil {
		fmt.Println("Error: Failed to compute CRC32:", err)
		return
	}
	fmt.Printf("Computed firmware CRC32: 0x%08X\n", firmwareCRC32)

	// Connect to TCP server
	var conn net.Conn
	for {
		conn, err = net.Dial("tcp", fmt.Sprintf("%s:%d", serverIP, SERVER_PORT))
		if err == nil {
			fmt.Printf("Connected to %s:%d\n", serverIP, SERVER_PORT)
			break
		}
		fmt.Printf("Connection failed: %v, retrying in 3 seconds...\n", err)
		time.Sleep(3 * time.Second)
	}
	defer conn.Close()

	// Send firmware size (4 bytes, network byte order)
	sizeBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(sizeBuffer, uint32(firmwareSize))
	conn.Write(sizeBuffer)

	// Initialize progress bar
	bar := progressbar.NewOptions(firmwareSize,
		progressbar.OptionSetWidth(50),
		progressbar.OptionSetDescription("Uploading..."),
		progressbar.OptionSetRenderBlankState(true),
	)

	// Send firmware data
	sentBytes := 0
	for i := 0; i < firmwareSize; i += BUFFER_SIZE {
		end := i + BUFFER_SIZE
		if end > firmwareSize {
			end = firmwareSize
		}
		chunk := firmwareData[i:end]
		conn.Write(chunk)

		// Wait for ACK
		ackBuffer := make([]byte, len(ACK_MSG))
		_, err := io.ReadFull(conn, ackBuffer)
		if err != nil || string(ackBuffer) != ACK_MSG {
			fmt.Println("Error: Transmission failed, incorrect ACK received")
			return
		}

		sentBytes += len(chunk)
		bar.Add(len(chunk))
	}

	// Send CRC32 checksum (4 bytes, network byte order)
	crcBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(crcBuffer, firmwareCRC32)
	conn.Write(crcBuffer)
	fmt.Printf("Firmware upload completed. Sent CRC32: 0x%08X\n", firmwareCRC32)

	// Receive CRC verification response from the server
	crcResponse := make([]byte, len(CRC_OK_MSG))
	_, err = io.ReadFull(conn, crcResponse)
	if err != nil || string(crcResponse) != CRC_OK_MSG {
		// Output error with red color
		fmt.Printf("\n")
		fmt.Printf("Error: CRC32 verification failed, firmware might be corrupted!\n")
	} else {
		// Output success with green color
		fmt.Printf("\n")
		fmt.Printf("Success: CRC32 verification passed, firmware transfer complete.\n")
	}
}

func isValidIP(ip string) bool {
	// Regular expression for validating an IPv4 address
	re := regexp.MustCompile(`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)
	return re.MatchString(ip)
}

func main() {
	// First check for version flag
	if len(os.Args) < 2 {
		fmt.Println("Usage: gboot <firmware_file> <server_ip>")
		fmt.Println("       gboot version  or  gboot -v  (to check version)")
		os.Exit(1)
	}

	arg := os.Args[1]

	// Check if the argument is 'version' or '-v'
	if arg == "version" || arg == "-v" {
		fmt.Printf("Firmware Sender Version: %s\n", VERSION)
		return
	}

	// Check if the argument is a firmware file and IP address
	if len(os.Args) < 3 {
		fmt.Println("Usage: gboot <firmware_file> <server_ip>")
		os.Exit(1)
	}

	// Validate IP address
	serverIP := os.Args[2]
	if !isValidIP(serverIP) {
		fmt.Printf("Error: Invalid IP address format: %s\n", serverIP)
		os.Exit(1)
	}

	// Send the firmware
	sendFirmware(arg, serverIP)
}
