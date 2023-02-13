package cmd

import (
	"context"
	"fmt"
	"github.com/corverroos/dvstore/app"
	"github.com/obolnetwork/charon/app/errors"
	"github.com/obolnetwork/charon/app/log"
	"github.com/obolnetwork/charon/app/z"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"net/url"
	"strings"
)

const (
	// The name of our config file, without the file extension because
	// viper supports many different config file languages.
	defaultConfigFilename = "dvstore"

	// The environment variable prefix of all environment variables bound to our command line flags.
	envPrefix = "dvstore"
)

// New returns a new root cobra command that handles our command line tool.
func New() *cobra.Command {
	return newRootCmd()
}

func newRootCmd() *cobra.Command {
	var conf app.Config
	root := &cobra.Command{
		Use:   "dvstore",
		Short: "DVStore - API for storing and retrieving DV cluster data",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initializeConfig(cmd)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := log.InitLogger(conf.Log); err != nil {
				return err
			}

			printFlags(cmd.Context(), cmd.Flags())

			return app.Run(cmd.Context(), conf)
		},
	}

	bindRunFlags(root.Flags(), &conf)
	bindLogFlags(root.Flags(), &conf.Log)

	titledHelp(root)

	return root
}

func bindRunFlags(flags *pflag.FlagSet, config *app.Config) {
	flags.StringVar(&config.HTTPAddress, "http-address", "localhost:8080", "HTTP server address")
}

func bindLogFlags(flags *pflag.FlagSet, config *log.Config) {
	flags.StringVar(&config.Format, "log-format", "console", "Log format; console, logfmt or json")
	flags.StringVar(&config.Level, "log-level", "info", "Log level; debug, info, warn or error")
}

// initializeConfig sets up the general viper config and binds the cobra flags to the viper flags.
func initializeConfig(cmd *cobra.Command) error {
	v := viper.New()

	v.SetConfigName(defaultConfigFilename)
	v.AddConfigPath(".")

	// Attempt to read the config file, gracefully ignoring errors
	// caused by a config file not being found. Return an error
	// if we cannot parse the config file.
	if err := v.ReadInConfig(); err != nil {
		// It's okay if there isn't a config file
		var cfgError viper.ConfigFileNotFoundError
		if ok := errors.As(err, &cfgError); !ok {
			return errors.Wrap(err, "read config")
		}
	}

	v.SetEnvPrefix(envPrefix)
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Bind the current command's flags to viper
	return bindFlags(cmd, v)
}

// bindFlags binds each cobra flag to its associated viper configuration (config file and environment variable).
func bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	var lastErr error

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Cobra provided flags take priority
		if f.Changed {
			return
		}

		// Define all the viper flag names to check
		viperNames := []string{
			f.Name,
			strings.ReplaceAll(f.Name, "_", "."), // TOML uses "." to indicate hierarchy, while we use "_" in this example.
		}

		for _, name := range viperNames {
			if !v.IsSet(name) {
				continue
			}

			val := v.Get(name)
			err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
			if err != nil {
				lastErr = err
			}

			break
		}
	})

	return lastErr
}

// titledHelp updates the command (and child commands) help flag usage to title case.
func titledHelp(cmd *cobra.Command) {
	cmd.InitDefaultHelpFlag()
	f := cmd.Flags().Lookup("help")
	f.Usage = strings.ToUpper(f.Usage[:1]) + f.Usage[1:]

	for _, child := range cmd.Commands() {
		titledHelp(child)
	}
}

// printFlags INFO logs all the given flags in alphabetical order.
func printFlags(ctx context.Context, flags *pflag.FlagSet) {
	ctx = log.WithTopic(ctx, "cmd")

	log.Info(ctx, "Parsed config", flagsToLogFields(flags)...)
}

// flagsToLogFields converts the given flags to log fields.
func flagsToLogFields(flags *pflag.FlagSet) []z.Field {
	var fields []z.Field
	flags.VisitAll(func(flag *pflag.Flag) {
		val := redact(flag.Name, flag.Value.String())

		if sliceVal, ok := flag.Value.(pflag.SliceValue); ok {
			var vals []string
			for _, s := range sliceVal.GetSlice() {
				vals = append(vals, redact(flag.Name, s))
			}
			val = "[" + strings.Join(vals, ",") + "]"
		}

		fields = append(fields, z.Str(flag.Name, val))
	})

	return fields
}

// redact returns a redacted version of the given flag value.
// It currently only supports redacting passwords in valid URLs provided in ".*address.*" flags.
func redact(flag, val string) string {
	if !strings.Contains(flag, "address") {
		return val
	}

	u, err := url.Parse(val)
	if err != nil {
		return val
	}

	return u.Redacted()
}
