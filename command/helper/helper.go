package helper

import "fmt"

// FlagDescriptor contains the description elements for a command flag
type FlagDescriptor struct {
	Description       string   // Flag description
	Arguments         []string // Arguments list
	ArgumentsOptional bool     // Flag indicating if flag arguments are optional
	FlagOptional      bool
}

// GetDescription ets the flag description
func (fd *FlagDescriptor) GetDescription() string {
	return fd.Description
}

// GetArgumentsList gets the list of arguments for the flag
func (fd *FlagDescriptor) GetArgumentsList() []string {
	return fd.Arguments
}

// AreArgumentsOptional checks if the flag arguments are optional
func (fd *FlagDescriptor) AreArgumentsOptional() bool {
	return fd.ArgumentsOptional
}

// IsFlagOptional checks if the flag itself is optional
func (fd *FlagDescriptor) IsFlagOptional() bool {
	return fd.FlagOptional
}

// GenerateHelp is a utility function called by every command's Help() method
func GenerateHelp(synopsys string, usage string, flagMap map[string]FlagDescriptor) string {
	helpOutput := ""

	flagCounter := 0
	for flagEl, descriptor := range flagMap {
		helpOutput += GenerateFlagDesc(flagEl, descriptor) + "\n"
		flagCounter++

		if flagCounter < len(flagMap) {
			helpOutput += "\n"
		}
	}

	if len(flagMap) > 0 {
		return fmt.Sprintf("Description:\n\n%s\n\nUsage:\n\n\t%s\n\nFlags:\n\n%s", synopsys, usage, helpOutput)
	} else {
		return fmt.Sprintf("Description:\n\n%s\n\nUsage:\n\n\t%s\n", synopsys, usage)
	}
}

// GenerateFlagDesc generates the flag descriptions in a readable format
func GenerateFlagDesc(flagEl string, descriptor FlagDescriptor) string {
	// Generate the top row (with various flags)
	topRow := fmt.Sprintf("--%s", flagEl)

	argumentsOptional := descriptor.AreArgumentsOptional()
	argumentsList := descriptor.GetArgumentsList()

	argLength := len(argumentsList)

	if argLength > 0 {
		topRow += " "
		if argumentsOptional {
			topRow += "["
		}

		for argIndx, argument := range argumentsList {
			topRow += argument

			if argIndx < argLength-1 && argLength > 1 {
				topRow += " "
			}
		}

		if argumentsOptional {
			topRow += "]"
		}
	}

	// Generate the bottom description
	bottomRow := fmt.Sprintf("\t%s", descriptor.GetDescription())

	return fmt.Sprintf("%s\n%s", topRow, bottomRow)
}

// GenerateUsage is a helper function for generating command usage text
func GenerateUsage(baseCommand string, flagMap map[string]FlagDescriptor) string {
	output := baseCommand + " "

	maxFlagsPerLine := 3 // Just an arbitrary value, can be anything reasonable

	var addedFlags int // Keeps track of when a newline character needs to be inserted
	for flagEl, descriptor := range flagMap {
		// Open the flag bracket
		if descriptor.IsFlagOptional() {
			output += "["
		}

		// Add the actual flag name
		output += fmt.Sprintf("--%s", flagEl)

		// Open the argument bracket
		if descriptor.AreArgumentsOptional() {
			output += " ["
		}

		argumentsList := descriptor.GetArgumentsList()

		// Add the flag arguments list
		for argIndex, argument := range argumentsList {
			if argIndex == 0 {
				// Only called for the first argument
				output += " "
			}

			output += argument

			if argIndex < len(argumentsList)-1 {
				output += " "
			}
		}

		// Close the argument bracket
		if descriptor.AreArgumentsOptional() {
			output += "]"
		}

		// Close the flag bracket
		if descriptor.IsFlagOptional() {
			output += "]"
		}

		addedFlags++
		if addedFlags%maxFlagsPerLine == 0 {
			output += "\n\t"
		} else {
			output += " "
		}
	}

	return output
}
