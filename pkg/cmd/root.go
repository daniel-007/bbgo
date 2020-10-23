package cmd

import (
	"os"
	"path"
	"strings"
	"time"

	"github.com/lestrrat-go/file-rotatelogs"
	"github.com/rifflock/lfshook"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/x-cray/logrus-prefixed-formatter"

	_ "github.com/go-sql-driver/mysql"
)

var RootCmd = &cobra.Command{
	Use:   "bbgo",
	Short: "bbgo trade bot",
	Long:  "bitcoin trader",

	// SilenceUsage is an option to silence usage when an error occurs.
	SilenceUsage: true,

	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

func init() {
	RootCmd.PersistentFlags().Bool("debug", false, "debug flag")

	// A flag can be 'persistent' meaning that this flag will be available to
	// the command it's assigned to as well as every command under that command.
	// For global flags, assign a flag as a persistent flag on the root.
	RootCmd.PersistentFlags().String("slack-token", "", "slack token")
	RootCmd.PersistentFlags().String("slack-trading-channel", "dev-bbgo", "slack trading channel")
	RootCmd.PersistentFlags().String("slack-error-channel", "bbgo-error", "slack error channel")

	RootCmd.PersistentFlags().String("binance-api-key", "", "binance api key")
	RootCmd.PersistentFlags().String("binance-api-secret", "", "binance api secret")

	RootCmd.PersistentFlags().String("max-api-key", "", "max api key")
	RootCmd.PersistentFlags().String("max-api-secret", "", "max api secret")
}

func Run() {
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Enable environment variable binding, the env vars are not overloaded yet.
	viper.AutomaticEnv()

	// setup the config paths for looking up the config file
	viper.AddConfigPath("config")
	viper.AddConfigPath("$HOME/.bbgo")
	viper.AddConfigPath("/etc/bbgo")

	// set the config file name and format for loading the config file.
	viper.SetConfigName("bbgo")
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		logrus.WithError(err).Fatal("failed to load config file")
	}

	// Once the flags are defined, we can bind config keys with flags.
	if err := viper.BindPFlags(RootCmd.PersistentFlags()); err != nil {
		logrus.WithError(err).Errorf("failed to bind persistent flags. please check the flag settings.")
	}

	if err := viper.BindPFlags(RootCmd.Flags()); err != nil {
		logrus.WithError(err).Errorf("failed to bind local flags. please check the flag settings.")
	}

	logrus.SetFormatter(&prefixed.TextFormatter{})

	logger := logrus.StandardLogger()
	if viper.GetBool("debug") {
		logger.SetLevel(logrus.DebugLevel)
	}

	environment := os.Getenv("BBGO_ENV")
	switch environment {
	case "production", "prod":

		writer, err := rotatelogs.New(
			path.Join("log", "access_log.%Y%m%d"),
			rotatelogs.WithLinkName("access_log"),
			// rotatelogs.WithMaxAge(24 * time.Hour),
			rotatelogs.WithRotationTime(time.Duration(24)*time.Hour),
		)
		if err != nil {
			logrus.Panic(err)
		}
		logger.AddHook(
			lfshook.NewHook(
				lfshook.WriterMap{
					logrus.DebugLevel: writer,
					logrus.InfoLevel:  writer,
					logrus.WarnLevel:  writer,
					logrus.ErrorLevel: writer,
					logrus.FatalLevel: writer,
				},
				&logrus.JSONFormatter{},
			),
		)
	}

	if err := RootCmd.Execute(); err != nil {
		logrus.WithError(err).Fatalf("cannot execute command")
	}
}