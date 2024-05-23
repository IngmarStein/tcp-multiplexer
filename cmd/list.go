/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"fmt"
	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "list application protocols that multiplexer supports",
	Run: func(cmd *cobra.Command, args []string) {
		for name := range message.Readers {
			fmt.Print("* ")
			fmt.Println(name)
		}

		fmt.Println("\nusage for example: ./tcp-multiplexer server -p echo")
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
