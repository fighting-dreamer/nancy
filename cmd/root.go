//
// Copyright 2018-present Sonatype Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package cmd

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"github.com/common-nighthawk/go-figure"
	"github.com/golang/dep"
	"github.com/mitchellh/go-homedir"
	"github.com/sirupsen/logrus"
	"github.com/sonatype-nexus-community/nancy/audit"
	"github.com/sonatype-nexus-community/nancy/buildversion"
	"github.com/sonatype-nexus-community/nancy/configuration"
	"github.com/sonatype-nexus-community/nancy/customerrors"
	. "github.com/sonatype-nexus-community/nancy/logger"
	"github.com/sonatype-nexus-community/nancy/ossindex"
	"github.com/sonatype-nexus-community/nancy/packages"
	"github.com/sonatype-nexus-community/nancy/parse"
	"github.com/sonatype-nexus-community/nancy/types"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/spf13/viper"
)

var cfgFile string

var configOssi configuration.Configuration
var excludeVulnerabilityFilePath string
var outputFormat string

var outputFormats = map[string]logrus.Formatter{
	"json":        &audit.JsonFormatter{},
	"json-pretty": &audit.JsonFormatter{PrettyPrint: true},
	"text":        &audit.AuditLogTextFormatter{Quiet: &configOssi.Quiet, NoColor: &configOssi.NoColor},
	"csv":         &audit.CsvFormatter{Quiet: &configOssi.Quiet},
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "nancy",
	Short: "Check for vulnerabilities in your Golang dependencies",
	Long: `nancy is a tool to check for vulnerabilities in your Golang dependencies,
powered by Sonatype OSS Index, and as well, works with Nexus IQ Server, allowing you
a smooth experience as a Golang developer, using the best tools in the market!`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		LogLady.Info("Nancy parsing config for OSS Index")
		//ossIndexConfig, err := configuration.Parse(args)
		err = completeConfig(&configOssi, args)
		if err != nil {
			flag.Usage()
			err = customerrors.ErrorExit{Err: err, Message: err.Error(), ExitCode: 1}
			return
		}
		if err = processConfig(configOssi); err != nil {
			return
		}
		LogLady.Info("Nancy finished parsing config for OSS Index")
		return
	},
	Args: cobra.ArbitraryArgs, // allows "deprecated" Gopkg.lock or go.sum path args
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	//rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.nancy.yaml)")

	// "help" cmd is built into Cobra
	//rootCmd.PersistentFlags().BoolVarP(&configOssi.Help, "help", "h", false, "provides help text on how to use nancy")

	rootCmd.PersistentFlags().CountVarP(&configOssi.LogLevel, "", "v", "Set log level, multiple v's is more verbose")

	rootCmd.PersistentFlags().BoolVarP(&configOssi.Quiet, "quiet", "q", false, "indicate output should contain only packages with vulnerabilities")
	rootCmd.PersistentFlags().BoolVar(&configOssi.Version, "version", false, "prints current nancy version")

	rootCmd.Flags().BoolVarP(&configOssi.NoColor, "no-color", "n", false, "indicate output should not be colorized")
	rootCmd.Flags().BoolVarP(&configOssi.CleanCache, "clean-cache", "c", false, "Deletes local cache directory")

	rootCmd.Flags().VarP(&configOssi.CveList, "exclude-vulnerability", "e", "Comma separated list of CVEs to exclude")
	rootCmd.Flags().StringVarP(&configOssi.Username, "user", "u", "", "Specify OSS Index username for request")
	rootCmd.Flags().StringVarP(&configOssi.Token, "token", "t", "", "Specify OSS Index API token for request")
	rootCmd.Flags().StringVarP(&excludeVulnerabilityFilePath, "exclude-vulnerability-file", "x", "./.nancy-ignore", "Path to a file containing newline separated CVEs to be excluded")
	rootCmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Styling for output format. "+fmt.Sprintf("%+q", reflect.ValueOf(outputFormats).MapKeys()))

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	//rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".nancy" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".nancy")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}

func completeConfig(config *configuration.Configuration, args []string) error {

	//var outputFormats = map[string]logrus.Formatter{
	//	"json":        &audit.JsonFormatter{},
	//	"json-pretty": &audit.JsonFormatter{PrettyPrint: true},
	//	"text":        &audit.AuditLogTextFormatter{Quiet: &config.Quiet, NoColor: &config.NoColor},
	//	"csv":         &audit.CsvFormatter{Quiet: &config.Quiet},
	//}
	//
	//	flag.BoolVar(&config.Help, "help", false, "provides help text on how to use nancy")
	//	flag.BoolVar(&config.NoColor, "no-color", false, "indicate output should not be colorized")
	//	flag.BoolVar(&config.Quiet, "quiet", false, "indicate output should contain only packages with vulnerabilities")
	//	flag.BoolVar(&config.Version, "version", false, "prints current nancy version")
	//	flag.BoolVar(&config.CleanCache, "clean-cache", false, "Deletes local cache directory")
	//	flag.BoolVar(&config.Info, "v", false, "Set log level to Info")
	//	flag.BoolVar(&config.Debug, "vv", false, "Set log level to Debug")
	//	flag.BoolVar(&config.Trace, "vvv", false, "Set log level to Trace")
	//	flag.Var(&config.CveList, "exclude-vulnerability", "Comma separated list of CVEs to exclude")
	//	flag.StringVar(&config.Username, "user", "", "Specify OSS Index username for request")
	//	flag.StringVar(&config.Token, "token", "", "Specify OSS Index API token for request")
	//	flag.StringVar(&excludeVulnerabilityFilePath, "exclude-vulnerability-file", "./.nancy-ignore", "Path to a file containing newline separated CVEs to be excluded")
	//	flag.StringVar(&outputFormat, "output", "text", "Styling for output format. "+fmt.Sprintf("%+q", reflect.ValueOf(outputFormats).MapKeys()))
	//
	//	flag.Usage = func() {
	//		_, _ = fmt.Fprintf(os.Stderr, `Usage:
	//	go list -m all | nancy [options]
	//	go list -m all | nancy iq [options]
	//	nancy config
	//	nancy [options] </path/to/Gopkg.lock>
	//	nancy [options] </path/to/go.sum>
	//
	//Options:
	//`)
	//		flag.PrintDefaults()
	//	}

	configuration.ConfigLocation = filepath.Join(configuration.HomeDir, types.OssIndexDirName, types.OssIndexConfigFileName)

	err := configuration.LoadConfigFromFile(configuration.ConfigLocation, config)
	if err != nil {
		LogLady.Info("Unable to load config from file")
	}

	//err = flag.CommandLine.Parse(args)
	//if err != nil {
	//	return config, err
	//}

	//modfilePath, err := getModfilePath()
	modfilePath, err := getModfilePathFromCmd(args)
	if err != nil {
		return err
	}
	if len(modfilePath) > 0 {
		config.Path = modfilePath
	} else {
		config.UseStdIn = true
	}

	if outputFormats[outputFormat] != nil {
		config.Formatter = outputFormats[outputFormat]
	} else {
		fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		fmt.Println("!!! Output format of", strings.TrimSpace(outputFormat), "is not valid. Defaulting to text output")
		fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		config.Formatter = outputFormats["text"]
	}

	err = configuration.GetCVEExcludesFromFile(config, excludeVulnerabilityFilePath)
	if err != nil {
		return err
	}

	return nil
}

func getModfilePathFromCmd(args []string) (modfilepath string, err error) {
	if len(args) > 0 {
		if len(args) != 1 {
			return modfilepath, fmt.Errorf("wrong number of modfile paths: %s", args)
		}
		return args[0], err
	}
	return modfilepath, err
}

func processConfig(config configuration.Configuration) (err error) {
	if config.Help {
		LogLady.Info("Printing usage and exiting clean")
		flag.Usage()
		os.Exit(0)
	}

	if config.Version {
		LogLady.WithFields(logrus.Fields{
			"build_time":   buildversion.BuildTime,
			"build_commit": buildversion.BuildCommit,
			"version":      buildversion.BuildVersion,
		}).Info("Printing version information and exiting clean")

		fmt.Println(buildversion.BuildVersion)
		_, _ = fmt.Printf("build time: %s\n", buildversion.BuildTime)
		_, _ = fmt.Printf("build commit: %s\n", buildversion.BuildCommit)
		err = customerrors.ErrorExit{ExitCode: 0}
		return
	}

	// @todo Change to use a switch statement
	if config.LogLevel == 1 || config.Info {
		LogLady.Level = logrus.InfoLevel
	}
	if config.LogLevel == 2 || config.Debug {
		LogLady.Level = logrus.DebugLevel
	}
	if config.LogLevel == 3 || config.Trace {
		LogLady.Level = logrus.TraceLevel
	}

	if config.CleanCache {
		LogLady.Info("Attempting to clean cache")
		if err := ossindex.RemoveCacheDirectory(); err != nil {
			LogLady.WithField("error", err).Error("Error cleaning cache")
			fmt.Printf("ERROR: cleaning cache: %v\n", err)
			os.Exit(1)
		}
		LogLady.Info("Cache cleaned")
		return
	}

	printHeader(!config.Quiet && reflect.TypeOf(config.Formatter).String() == "*audit.AuditLogTextFormatter")

	if config.UseStdIn {
		LogLady.Info("Parsing config for StdIn")
		if err = doStdInAndParse(config); err != nil {
			return
		}
	}
	if !config.UseStdIn {
		LogLady.Info("Parsing config for file based scan")
		err = doCheckExistenceAndParse(config)
	}

	return
}

func printHeader(print bool) {
	if print {
		LogLady.Info("Attempting to print header")
		figure.NewFigure("Nancy", "larry3d", true).Print()
		figure.NewFigure("By Sonatype & Friends", "pepper", true).Print()

		LogLady.WithField("version", buildversion.BuildVersion).Info("Printing Nancy version")
		fmt.Println("Nancy version: " + buildversion.BuildVersion)
		LogLady.Info("Finished printing header")
	}
}

func doStdInAndParse(config configuration.Configuration) (err error) {
	LogLady.Info("Beginning StdIn parse for OSS Index")
	if err = checkStdIn(); err != nil {
		return err
	}
	LogLady.Info("Instantiating go.mod package")

	mod := packages.Mod{}
	scanner := bufio.NewScanner(os.Stdin)

	LogLady.Info("Beginning to parse StdIn")
	mod.ProjectList, _ = parse.GoList(scanner)
	LogLady.WithFields(logrus.Fields{
		"projectList": mod.ProjectList,
	}).Debug("Obtained project list")

	var purls = mod.ExtractPurlsFromManifest()
	LogLady.WithFields(logrus.Fields{
		"purls": purls,
	}).Debug("Extracted purls")

	LogLady.Info("Auditing purls with OSS Index")
	err = checkOSSIndex(purls, nil, config)

	return err
}

func doCheckExistenceAndParse(config configuration.Configuration) error {
	switch {
	case strings.Contains(config.Path, "Gopkg.lock"):
		workingDir := filepath.Dir(config.Path)
		if workingDir == "." {
			workingDir, _ = os.Getwd()
		}
		getenv := os.Getenv("GOPATH")
		ctx := dep.Ctx{
			WorkingDir: workingDir,
			GOPATHs:    []string{getenv},
		}
		project, err := ctx.LoadProject()
		if err != nil {
			return customerrors.NewErrorExitPrintHelp(err, fmt.Sprintf("could not read lock at path %s", config.Path))
		}
		if project.Lock == nil {
			return customerrors.NewErrorExitPrintHelp(errors.New("dep failed to parse lock file and returned nil"), "nancy could not continue due to dep failure")
		}

		purls, invalidPurls := packages.ExtractPurlsUsingDep(project)

		if err := checkOSSIndex(purls, invalidPurls, config); err != nil {
			return err
		}
	case strings.Contains(config.Path, "go.sum"):
		mod := packages.Mod{}
		mod.GoSumPath = config.Path
		manifestExists, err := mod.CheckExistenceOfManifest()
		if err != nil {
			return err
		}
		if manifestExists {
			mod.ProjectList, _ = parse.GoSum(config.Path)
			var purls = mod.ExtractPurlsFromManifest()

			if err := checkOSSIndex(purls, nil, config); err != nil {
				return err
			}
		}
	default:
		//os.Exit(3)
		return customerrors.ErrorExit{ExitCode: 3, Message: fmt.Sprintf("invalid path arg: %s", config.Path)}
	}
	return nil
}

func checkOSSIndex(purls []string, invalidpurls []string, config configuration.Configuration) error {
	var packageCount = len(purls)
	coordinates, err := ossindex.AuditPackagesWithOSSIndex(purls, &config)
	if err != nil {
		return customerrors.NewErrorExitPrintHelp(err, "Error auditing packages")
	}

	var invalidCoordinates []types.Coordinate
	for _, invalidpurl := range invalidpurls {
		invalidCoordinates = append(invalidCoordinates, types.Coordinate{Coordinates: invalidpurl, InvalidSemVer: true})
	}

	if count := audit.LogResults(config.Formatter, packageCount, coordinates, invalidCoordinates, config.CveList.Cves); count > 0 {
		os.Exit(count)
	}
	return nil
}

var stdInInvalid = customerrors.ErrorExit{ExitCode: 1, Message: "StdIn is invalid, either empty or another reason"}

func checkStdIn() (err error) {
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		LogLady.Info("StdIn is valid")
	} else {
		LogLady.Error(stdInInvalid.Message)
		flag.Usage()
		err = stdInInvalid
	}
	return
}
