/*
Copyright © 2019 National Library of Norway

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
	"github.com/fsnotify/fsnotify"
	"github.com/nlnwa/gowarc/cmd/warc/cmd/cat"
	"github.com/nlnwa/gowarc/cmd/warc/cmd/index"
	"github.com/nlnwa/gowarc/cmd/warc/cmd/ls"
	"github.com/nlnwa/gowarc/cmd/warc/cmd/serve"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type conf struct {
	cfgFile string
}

// NewCommand returns a new cobra.Command implementing the root command for warc
func NewCommand() *cobra.Command {
	c := &conf{}
	cmd := &cobra.Command{
		Use:   "warc",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		// Uncomment the following line if your bare application
		// has an action associated with it:
		//	Run: func(cmd *cobra.Command, args []string) { },
	}

	cobra.OnInitialize(func() { c.initConfig() })

	// Flags
	cmd.PersistentFlags().StringVar(&c.cfgFile, "config", "", "config file. If not set, /etc/warc/, $HOME/.warc/ and current working dir will be searched for file config.yaml")
	viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config"))

	// Subcommands
	cmd.AddCommand(ls.NewCommand())
	cmd.AddCommand(cat.NewCommand())
	cmd.AddCommand(serve.NewCommand())
	cmd.AddCommand(index.NewCommand())

	return cmd
}

// initConfig reads in config file and ENV variables if set.
func (c *conf) initConfig() {
	viper.SetTypeByDefaultValue(true)
	viper.SetDefault("warcdir", []string{"."})
	viper.SetDefault("indexdir", ".")
	viper.SetDefault("autoindex", true)

	viper.AutomaticEnv() // read in environment variables that match

	if viper.IsSet("config") {
		// Use config file from the flag.
		viper.SetConfigFile(viper.GetString("config"))
	} else {
		// Search config in home directory with name ".warc" (without extension).
		viper.SetConfigName("config")      // name of config file (without extension)
		viper.SetConfigType("yaml")        // REQUIRED if the config file does not have the extension in the name
		viper.AddConfigPath("/etc/warc/")  // path to look for the config file in
		viper.AddConfigPath("$HOME/.warc") // call multiple times to add many search paths
		viper.AddConfigPath(".")           // optionally look for config in the working directory
	}

	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		fmt.Println("Config file changed:", e.Name)
	})

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error
		} else {
			// Config file was found but another error was produced
			log.Fatalf("error reading config file: %v", err)
		}
	}

	// Config file found and successfully parsed
	fmt.Println("Using config file:", viper.ConfigFileUsed())
}
