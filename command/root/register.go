package root

import "github.com/0xPolygon/polygon-edge/command/version"

func (rc *RootCommand) registerCommands() {
	rc.registerBackupCommands()
	rc.registerDevCommands()
	rc.registerGenesisCommands()
	rc.registerIBFTCommands()
	rc.registerLoadbotCommands()

	rc.baseCmd.AddCommand(version.NewVersionCommand())
}

func (rc *RootCommand) registerBackupCommands() {

}

func (rc *RootCommand) registerDevCommands() {

}

func (rc *RootCommand) registerGenesisCommands() {

}

func (rc *RootCommand) registerIBFTCommands() {

}

func (rc *RootCommand) registerLoadbotCommands() {

}
