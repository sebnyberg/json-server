// Package cmd contains the functionality for the set of commands
// currently supported by the CLI.
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"time"

	"github.com/spf13/cobra"

	"github.com/chanioxaris/json-server/handler"
	"github.com/chanioxaris/json-server/logger"
)

// rootCmd represents the base command when called without any sub commands.
var rootCmd = &cobra.Command{
	Use:   "json-server",
	Short: "Create a dummy REST API from a json file with zero coding within seconds",
	Long: `json-server is a cross-platform CLI tool to create within seconds a dummy REST API from a provided json 
			file. Depending on the provided file some default http endpoints are created. For array data (plural) a GET, 
			GET by ID, POST, PUT by ID, PATCH by ID and DELETE by ID endpoints are available. For object data (singular) 
			a GET endpoint is available`,
	RunE: run,
}

var (
	errFailedParseFlag   = errors.New("failed to parse flag")
	errFailedParseFile   = errors.New("failed to parse file")
	errFileNotFound      = errors.New("unable to find requested file")
	errFailedStartServer = errors.New("failed to start JSON server. Maybe port already in use")
)

func init() {
	// Optional flag to set the server port.
	rootCmd.Flags().StringP("port", "p", "3000", "Port the server will listen to")
	// Optional flag to set the watch file.
	rootCmd.Flags().StringP("file", "f", "db.json", "File to watch")
	// Optional flag to enable logs.
	rootCmd.Flags().BoolP("logs", "l", false, "Enable logs")
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, _ []string) error {
	rand.Seed(time.Now().UnixNano())

	// Parse command's flags.
	port, err := cmd.Flags().GetString("port")
	if err != nil {
		return fmt.Errorf("%w: port", errFailedParseFlag)
	}

	file, err := cmd.Flags().GetString("file")
	if err != nil {
		return fmt.Errorf("%w: file", errFailedParseFlag)
	}

	logs, err := cmd.Flags().GetBool("logs")
	if err != nil {
		return fmt.Errorf("%w: logs", errFailedParseFlag)
	}

	// Setup logger.
	logger.Setup(logs)

	// Get storage resources.
	storageResources, err := getStorageResources(file)
	if err != nil {
		return err
	}

	// Setup API handler.
	apiHandler, err := handler.Setup(storageResources, file)
	if err != nil {
		return err
	}

	api := &http.Server{
		Addr:    ":" + port,
		Handler: apiHandler,
		// Good practice to set timeouts to avoid Slowloris attacks.
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
	}

	// Start REST API server.
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return errFailedStartServer
	}

	go api.Serve(listener)

	// Display info about available resources and home page.
	displayInfo(storageResources, port)

	gracefulShutdown(api)

	return nil
}

// gracefulShutdown handles any signal that interrupts the running server
func gracefulShutdown(server *http.Server) {
	c := make(chan os.Signal, 1)
	// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	signal.Notify(c, os.Interrupt)

	// Block until we receive our signal.
	<-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	if err := server.Shutdown(ctx); err != nil {
		fmt.Println("failed to gracefully shutdown server")
		return
	}

	fmt.Println("gracefully shutting down server")
}

func getStorageResources(filename string) (map[string]bool, error) {
	// Read file contents used as storage.
	contentBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", errFileNotFound, filename)
	}

	content := map[string]interface{}{}
	if err = json.Unmarshal(contentBytes, &content); err != nil {
		return nil, fmt.Errorf("%w: %s", errFailedParseFile, filename)
	}

	storageKeys := make(map[string]bool)

	// Range on content to retrieve resource keys and type (plural, singular).
	for resource, data := range content {
		switch reflect.TypeOf(data).Kind() {
		case reflect.Slice:
			storageKeys[resource] = false
		default:
			storageKeys[resource] = true
		}
	}

	return storageKeys, nil
}

func displayInfo(storageResources map[string]bool, port string) {
	fmt.Println("JSON Server successfully running")
	fmt.Println()

	fmt.Println("Resources")
	for resource := range storageResources {
		fmt.Printf("http://localhost:%s/%s\n", port, resource)
	}

	fmt.Printf("http://localhost:%s/db\n", port)
	fmt.Println()

	fmt.Println("Home")
	fmt.Printf("http://localhost:%s\n", port)
	fmt.Println()
}
