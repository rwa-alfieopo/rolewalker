package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ConfirmAction prompts the user for confirmation with a custom message
// Returns true if user types 'yes', false otherwise
func ConfirmAction(message string) bool {
	fmt.Print(message)
	
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes"
}

// ConfirmDatabaseRestore prompts for confirmation before database restore
func ConfirmDatabaseRestore(env, inputFile string) bool {
	message := fmt.Sprintf(`
⚠️  WARNING: You are about to restore a database backup!
   Environment: %s
   Input file:  %s

   This operation may overwrite existing data.

   Type 'yes' to confirm: `, env, inputFile)
	
	return ConfirmAction(message)
}

// ConfirmReplicationSwitch prompts for confirmation before Blue-Green switchover
func ConfirmReplicationSwitch(deploymentName, source, target string) bool {
	message := fmt.Sprintf(`
⚠️  WARNING: You are about to perform a Blue-Green switchover!
   Deployment: %s
   Source:     %s
   Target:     %s

   This will switch production traffic to the target cluster.
   Type 'yes' to confirm: `, deploymentName, source, target)
	
	return ConfirmAction(message)
}

// ConfirmReplicationCreate prompts for confirmation before creating deployment
func ConfirmReplicationCreate(name, source string) bool {
	message := fmt.Sprintf(`
⚠️  Creating a new Blue-Green deployment:
   Name:   %s
   Source: %s

   This will create a clone of the source cluster.
   Type 'yes' to confirm: `, name, source)
	
	return ConfirmAction(message)
}

// ConfirmReplicationDelete prompts for confirmation before deleting deployment
func ConfirmReplicationDelete(deploymentName string, deleteTarget bool) bool {
	targetWarning := ""
	if deleteTarget {
		targetWarning = "\n   ⚠️  Target cluster will also be DELETED!"
	}
	
	message := fmt.Sprintf(`
⚠️  WARNING: You are about to delete a Blue-Green deployment!
   Deployment: %s%s

   Type 'yes' to confirm: `, deploymentName, targetWarning)
	
	return ConfirmAction(message)
}
// IsProductionEnvironment checks if the given environment is a production environment
func IsProductionEnvironment(env string) bool {
	prodEnvs := []string{"prod", "preprod", "trg", "live"}
	envLower := strings.ToLower(env)
	
	for _, prodEnv := range prodEnvs {
		if envLower == prodEnv {
			return true
		}
	}
	
	return false
}

// ConfirmProductionOperation prompts for confirmation before executing operations in production
// Returns true if user types 'yes', false otherwise
func ConfirmProductionOperation(env, operation string) bool {
	if !IsProductionEnvironment(env) {
		return true // No confirmation needed for non-production
	}
	
	// ANSI color codes
	const (
		redBg     = "\033[41m"  // Red background
		whiteFg   = "\033[97m"  // White foreground
		bold      = "\033[1m"   // Bold text
		reset     = "\033[0m"   // Reset all formatting
		redFg     = "\033[31m"  // Red foreground
	)
	
	// Print warning with red background
	fmt.Printf("\n%s%s%s", redBg, whiteFg, bold)
	fmt.Printf("                                                                    ")
	fmt.Printf("%s\n", reset)
	
	fmt.Printf("%s%s%s", redBg, whiteFg, bold)
	fmt.Printf("  🚨  PRODUCTION ENVIRONMENT DETECTED  🚨                           ")
	fmt.Printf("%s\n", reset)
	
	fmt.Printf("%s%s%s", redBg, whiteFg, bold)
	fmt.Printf("                                                                    ")
	fmt.Printf("%s\n\n", reset)
	
	fmt.Printf("%s%sEnvironment:%s %s\n", bold, redFg, reset, strings.ToUpper(env))
	fmt.Printf("%s%sOperation:%s   %s\n\n", bold, redFg, reset, operation)
	
	fmt.Println("You are about to perform an operation in a PRODUCTION environment.")
	fmt.Println("Please ensure you have proper authorization and have reviewed the changes.")
	fmt.Printf("\n%s%sType 'yes' to confirm:%s ", bold, redFg, reset)
	
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes"
}

// SelectFromList prompts the user to select an item from a list
// Returns the selected item and true, or empty string and false if cancelled
func SelectFromList(prompt string, items []string) (string, bool) {
	if len(items) == 0 {
		return "", false
	}

	fmt.Println(prompt)
	fmt.Println(strings.Repeat("-", 60))

	for i, item := range items {
		fmt.Printf("  [%d] %s\n", i+1, item)
	}

	fmt.Print("\nSelect number (or 'q' to quit): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", false
	}

	response = strings.TrimSpace(strings.ToLower(response))

	if response == "q" || response == "quit" || response == "exit" {
		return "", false
	}

	var selection int
	if _, err := fmt.Sscanf(response, "%d", &selection); err != nil {
		fmt.Println("Invalid selection")
		return "", false
	}

	if selection < 1 || selection > len(items) {
		fmt.Println("Selection out of range")
		return "", false
	}

	return items[selection-1], true
}

