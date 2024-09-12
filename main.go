package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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

	// Define the /os endpoint
	r.GET("/os", func(c *gin.Context) {
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
			cmd = exec.Command("systemctl", "status", "--failed", "--no-pager")
		} else {
			cmd = exec.Command("systemctl", "status", "--no-pager")
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

	// Define the /packages GET endpoint that returns a list of installed packages
	r.GET("/packages", func(c *gin.Context) {
		osReleaseData, err := readOSReleaseFile("/etc/os-release")
		if err != nil {
			c.JSON(500, gin.H{"error": "Unable to determine the operating system"})
			return
		}

		var cmd *exec.Cmd
		switch osReleaseData["ID"] {
		case "ubuntu", "debian":
			cmd = exec.Command("dpkg-query", "-W", "-f=${binary:Package}\n")
		case "fedora", "centos", "rhel":
			cmd = exec.Command("dnf", "list", "installed")
		default:
			c.JSON(400, gin.H{"error": "Unsupported operating system"})
			return
		}

		output, err := cmd.Output()
		if err != nil {
			c.JSON(500, gin.H{"error": "Failed to get installed packages", "output": err.Error()})
			return
		}

		// Parse the output into a list of packages
		packageList := strings.Split(strings.TrimSpace(string(output)), "\n")
		c.JSON(200, gin.H{"installed_packages": packageList})
	})

	// Define the /binaries endpoint to count binaries in $PATH
	r.GET("/binaries", func(c *gin.Context) {
		// Get the $PATH environment variable
		pathEnv := os.Getenv("PATH")
		if pathEnv == "" {
			c.JSON(500, gin.H{"error": "$PATH environment variable is empty"})
			return
		}

		// Split $PATH into directories
		dirs := strings.Split(pathEnv, ":")

		// Count binaries in each directory
		binaryCount := 0
		for _, dir := range dirs {
			files, err := os.ReadDir(dir)
			if err != nil {
				continue // Skip directories we can't read
			}
			for _, file := range files {
				// Check if it's an executable file
				if !file.IsDir() {
					pathToFile := filepath.Join(dir, file.Name())
					if isExecutable(pathToFile) {
						binaryCount++
					}
				}
			}
		}

		c.JSON(200, gin.H{"binary_count": binaryCount})
	})

	// Define the /kubernetes GET endpoint to check if Kubernetes is installed
	r.GET("/kubernetes", func(c *gin.Context) {
		isInstalled := checkKubernetesInstallation()
		c.JSON(200, gin.H{
			"installed": isInstalled,
		})
	})

	// Define the /kubernetes POST endpoint
	r.POST("/kubernetes", func(c *gin.Context) {
		output, err := installAndBootstrapKubernetes()
		if err != nil {
			c.JSON(500, gin.H{
				"error":   "Failed to install and bootstrap Kubernetes",
				"details": err.Error(),
				"output":  output,
			})
			return
		}
		c.JSON(200, gin.H{
			"message": "Kubernetes successfully installed and bootstrapped",
			"output":  output,
		})
	})

	// Start the Gin server
	r.Run(":80") // Default runs on :8080
}

// Function to check if Kubernetes is installed on the system
func checkKubernetesInstallation() bool {
	// Check if kubeadm is installed
	_, errKubeadm := exec.LookPath("kubeadm")
	// Check if kubectl is installed
	_, errKubectl := exec.LookPath("kubectl")
	// Check if kubelet is installed
	_, errKubelet := exec.LookPath("kubelet")

	// If all are installed, return true
	if errKubeadm == nil && errKubectl == nil && errKubelet == nil {
		return true
	}

	// Otherwise, return false
	return false
}

// Function to install and bootstrap Kubernetes on Ubuntu
func installAndBootstrapKubernetes() (string, error) {
	var outputBuffer bytes.Buffer

	// Commands to install Kubernetes dependencies
	commands := []string{
		// Update and install dependencies
		"sudo apt-get update",
		"sudo apt-get install -y apt-transport-https ca-certificates curl",
		"curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -",
		`sudo bash -c 'cat <<EOF >/etc/apt/sources.list.d/kubernetes.list
deb https://apt.kubernetes.io/ kubernetes-xenial main
EOF'`,
		"sudo apt-get update",

		// Install kubeadm, kubelet, and kubectl
		"sudo apt-get install -y kubelet kubeadm kubectl",

		// Disable swap
		"sudo swapoff -a",

		// Initialize the Kubernetes cluster with kubeadm
		"sudo kubeadm init",

		// Setup kubectl for the ubuntu user
		"mkdir -p $HOME/.kube",
		"sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config",
		"sudo chown $(id -u):$(id -g) $HOME/.kube/config",

		// Install a pod network (flannel or weave)
		"kubectl apply -f https://raw.githubusercontent.com/coreos/flannel/master/Documentation/kube-flannel.yml",
	}

	// Execute each command and collect the output
	for _, cmd := range commands {
		fmt.Printf("Running command: %s\n", cmd) // Print command being executed
		log.Printf("Executing: %s", cmd)
		if err := execCommand(cmd, &outputBuffer); err != nil {
			fmt.Printf("Error during command execution: %s\n", err)
			return outputBuffer.String(), fmt.Errorf("failed to execute: %s", cmd)
		}
	}

	return outputBuffer.String(), nil
}

// Helper function to execute a shell command and capture its output
func execCommand(cmd string, outputBuffer *bytes.Buffer) error {
	command := exec.Command("bash", "-c", cmd)
	command.Stdout = outputBuffer
	command.Stderr = outputBuffer

	// Execute the command and capture stdout/stderr
	err := command.Run()

	// Print the output to the application stdout
	fmt.Printf("Output of command '%s':\n%s\n", cmd, outputBuffer.String())
	return err
}

// Helper function to check if a file is executable
func isExecutable(filePath string) bool {
	info, err := os.Stat(filePath)
	if err != nil {
		return false
	}
	mode := info.Mode()
	return mode&0111 != 0 // Check if any of the execute bits are set
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
