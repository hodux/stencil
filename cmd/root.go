// cmd/root.go
package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "stencil",
	Short: "Turn your dynamic collaborative environment (Outline Wiki) into a ready to serve static doccis site (Hextra)",
}

func Execute() {
	if err := rootCmd.Execute(); 
	err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}
