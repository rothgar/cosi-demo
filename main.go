package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
)

type PackageConfig struct {
	Packages struct {
		Installed   []string `yaml:"installed"`
		Uninstalled []string `yaml:"uninstalled"`
	} `yaml:"packages"`
}

func main() {
	r := gin.Default()

	// Define the /os-release endpoint
	r.GET("/os-release", func(c *gin.Context) {
		data, err := readOSReleaseFile("/etc/os-release")
		if err != nil {
			c.JSON(500, gin.H{"error": "Unable to read /etc/os-release file"})
			return
		}
		c.JSON(200, data)
	})

	// Define the /uname endpoint
	r.GET("/uname", func(c *gin.Context) {
		output, err := getUnameOutput()
		if err != nil {
			c.JSON(500, gin.H{"error": "Unable to get uname output"})
			return
		}
		c.JSON(200, output)
	})

	// Define the /systemctl/status endpoint
	r.POST("/systemctl/status", func(c *gin.Context) {
		var request struct {
			Failed bool `json:"failed"`
		}

		// Parse the incoming JSON request
		if err := c.BindJSON(&request); err != nil {
			c.JSON(400, gin.H{"error": "Invalid request format"})
			return
		}

		// Prepare the systemctl command based on the request
		var cmd *exec.Cmd
		if request.Failed {
			cmd = exec.Command("systemctl", "status", "--failed", "--nop-pager", "--output", "json")
		} else {
			cmd = exec.Command("systemctl", "status", "--no-pager", "--output", "json")
		}

		// Execute the command
		output, err := cmd.Output()
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to execute systemctl command"})
			return
		}

		// Parse the command output as JSON
		var jsonResponse interface{}
		if err := json.Unmarshal(output, &jsonResponse); err != nil {
			c.JSON(500, gin.H{"error": "Failed to parse JSON output from systemctl"})
			return
		}

		// Return the parsed JSON response
		c.JSON(200, jsonResponse)
	})

	// Define the /packages endpoint that accepts a YAML file
	r.POST("/packages", func(c *gin.Context) {
		// Read the YAML file
		var packageConfig PackageConfig
		if err := c.ShouldBindYAML(&packageConfig); err != nil {
			c.JSON(400, gin.H{"error": "Invalid YAML format"})
			return
		}

		// Determine the OS and package manager
		osReleaseData, err := readOSReleaseFile("/etc/os-release")
		if err != nil {
			c.JSON(500, gin.H{"error": "Unable to determine the operating system"})
			return
		}

		var installCmd, uninstallCmd *exec.Cmd
		switch osReleaseData["ID"] {
		case "ubuntu", "debian":
			if len(packageConfig.Packages.Installed) > 0 {
				installCmd = exec.Command("apt-get", append([]string{"install", "-y"}, packageConfig.Packages.Installed...)...)
			}
			if len(packageConfig.Packages.Uninstalled) > 0 {
				uninstallCmd = exec.Command("apt-get", append([]string{"remove", "-y"}, packageConfig.Packages.Uninstalled...)...)
			}
		case "fedora", "centos", "rhel":
			if len(packageConfig.Packages.Installed) > 0 {
				installCmd = exec.Command("dnf", append([]string{"install", "-y"}, packageConfig.Packages.Installed...)...)
			}
			if len(packageConfig.Packages.Uninstalled) > 0 {
				uninstallCmd = exec.Command("dnf", append([]string{"remove", "-y"}, packageConfig.Packages.Uninstalled...)...)
			}
		default:
			c.JSON(400, gin.H{"error": "Unsupported operating system"})
			return
		}

		// Execute the installation and uninstallation commands
		var installOutput, uninstallOutput bytes.Buffer
		if installCmd != nil {
			installCmd.Stdout = &installOutput
			installCmd.Stderr = &installOutput
			if err := installCmd.Run(); err != nil {
				c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to install packages: %v", err), "output": installOutput.String()})
				return
			}
		}

		if uninstallCmd != nil {
			uninstallCmd.Stdout = &uninstallOutput
			uninstallCmd.Stderr = &uninstallOutput
			if err := uninstallCmd.Run(); err != nil {
				c.JSON(500, gin.H{"error": fmt.Sprintf("Failed to uninstall packages: %v", err), "output": uninstallOutput.String()})
				return
			}
		}

		c.JSON(200, gin.H{
			"install_output":   installOutput.String(),
			"uninstall_output": uninstallOutput.String(),
		})
	})

	// Start the Gin server
	r.Run() // Default runs on :8080
}

// Function to read and parse the /etc/os-release file
func readOSReleaseFile(filePath string) (map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	result := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Ignore comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		// Split the line by the first '=' character
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), `"`) // Remove surrounding quotes
			result[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

// Function to execute the `uname -a` command and return its output with labeled fields
func getUnameOutput() (map[string]string, error) {
	kernelNameCmd := exec.Command("uname")
	kernelNameOutput, err := kernelNameCmd.Output()
	if err != nil {
		return nil, err
	}
	nodeNameCmd := exec.Command("uname", "-n")
	nodeNameOutput, err := nodeNameCmd.Output()
	if err != nil {
		return nil, err
	}
	kernelReleaseCmd := exec.Command("uname", "-r")
	kernelReleaseOutput, err := kernelReleaseCmd.Output()
	if err != nil {
		return nil, err
	}
	kernelVersionCmd := exec.Command("uname", "-v")
	kernelVersionOutput, err := kernelVersionCmd.Output()
	if err != nil {
		return nil, err
	}
	machineCmd := exec.Command("uname", "-m")
	machineOutput, err := machineCmd.Output()
	if err != nil {
		return nil, err
	}
	processorCmd := exec.Command("uname", "-p")
	processorOutput, err := processorCmd.Output()
	if err != nil {
		return nil, err
	}
	hardwareCmd := exec.Command("uname", "-i")
	hardwareOutput, err := hardwareCmd.Output()
	if err != nil {
		return nil, err
	}
	osCmd := exec.Command("uname", "-o")
	osOutput, err := osCmd.Output()
	if err != nil {
		return nil, err
	}

	// Map the fields to labels
	result := map[string]string{
		"kernel_name":    strings.TrimSpace(string(kernelNameOutput)),
		"nodename":       strings.TrimSpace(string(nodeNameOutput)),
		"kernel_release": strings.TrimSpace(string(kernelReleaseOutput)),
		"kernel_version": strings.TrimSpace(string(kernelVersionOutput)),
		"machine":        strings.TrimSpace(string(machineOutput)),
		"processor":      strings.TrimSpace(string(processorOutput)),
		"hardware":       strings.TrimSpace(string(hardwareOutput)),
		"os":             strings.TrimSpace(string(osOutput)),
	}

	return result, nil
}
