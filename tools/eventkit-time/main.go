package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/jtolio/eventkit"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/zeebo/errs/v2"
	"golang.org/x/sync/errgroup"
)

func main() {
	c := cobra.Command{
		Use:   "eventkit-time [--tag=value] command args",
		Short: "CLI tool for performance tests, like `time` but results are sent to an eventkit target",
	}
	_ = c.Flags().StringP("name", "n", "test", "Name of the event sending out")
	_ = c.Flags().StringP("destination", "d", "localhost:9000", "UDP host and port to send out package")
	_ = c.Flags().StringSliceP("tag", "t", []string{}, "Custom tags to add to the events")
	_ = c.Flags().StringP("instance", "i", "", "Instance name of the eventkitd monitoring (default: hostname)")
	_ = c.Flags().StringP("scope", "s", "eventkit-time", "Scope to use for events")
	version := c.Flags().BoolP("version", "v", false, "Scope to use for events")
	viper.SetConfigName("eventkit-time")
	viper.SetEnvPrefix("EVENTKIT")
	viper.AutomaticEnv()
	err := viper.BindPFlags(c.Flags())
	if err != nil {
		panic(err)
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if *version {
			if bi, ok := debug.ReadBuildInfo(); ok {
				for _, s := range bi.Settings {
					if strings.HasPrefix(s.Key, "vcs.") {
						fmt.Printf("%+v\n", s.Key+"="+s.Value)
					}
				}
			}
			return nil
		}
		if len(args) == 0 {
			return errs.Errorf("Command is missing")
		}
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return err
			}
		}

		return execute(viper.GetString("destination"), viper.GetString("name"), args, viper.GetStringSlice("tag"), viper.GetString("scope"), viper.GetString("instance"))
	}
	err = c.Execute()
	if err != nil {
		log.Fatalf("%++v", err)
	}
}

func execute(dest string, name string, args []string, customTags []string, scope string, instance string) error {
	ek := eventkit.DefaultRegistry.Scope(scope)
	if instance == "" {
		instance, _ = os.Hostname()
		if instance == "" {
			instance = "unknown"
		}
	}

	client := eventkit.NewUDPClient("eventkit-time", "0.0.1", instance, dest)
	eventkit.DefaultRegistry.AddDestination(client)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := errgroup.Group{}
	w.Go(func() error {
		client.Run(ctx)
		return nil
	})

	var cmd = exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	start := time.Now()
	err := cmd.Run()
	if err != nil {
		return err
	}
	usage := syscall.Rusage{}
	err = syscall.Getrusage(syscall.RUSAGE_CHILDREN, &usage)
	if err != nil {
		return errs.Wrap(err)
	}
	t := time.Since(start)

	var tags []eventkit.Tag
	tags = append(tags, eventkit.String("cmd", args[0]))
	tags = append(tags, eventkit.Int64("duration-ms", t.Milliseconds()))
	tags = append(tags, eventkit.Int64("user-time-ms", usage.Utime.Nano()/1000000))
	tags = append(tags, eventkit.Int64("system-time-ms", usage.Stime.Nano()/1000000))
	tags = append(tags, eventkit.Int64("max-rss", usage.Maxrss))
	for _, c := range customTags {
		parts := strings.SplitN(c, "=", 2)
		tags = append(tags, eventkit.String(parts[0], parts[1]))
	}
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if strings.HasPrefix(parts[0], "EVENTKIT_TAG_") {
			tagName := strings.TrimPrefix(parts[0], "EVENTKIT_TAG_")
			tagName = strings.ToLower(tagName)
			tags = append(tags, eventkit.String(tagName, parts[1]))
		}
	}
	ek.Event(name, tags...)
	cancel()
	return w.Wait()
}
