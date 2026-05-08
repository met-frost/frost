package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sort"
	"strconv"
	"syscall"
	"time"

	_ "expvar"
	_ "net/http/pprof"

	"github.com/felixge/fgprof"
	"gitlab.met.no/frost/frost/internal/auth"
	"gitlab.met.no/frost/frost/internal/common"
	"gitlab.met.no/frost/frost/internal/frost"
	localhttp "gitlab.met.no/frost/frost/internal/http"
	obs "gitlab.met.no/frost/frost/internal/routes/api/insituobs"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/storagebackends"
	"gitlab.met.no/frost/frost/internal/routes/api/insituobs/tsregistry"
	"gitlab.met.no/frost/frost/pkg/metaservice"
)

func printNonFatalErrors(errors []error) {

	if len(errors) == 0 {
		log.Printf("found no non-fatal errors")
		return
	}

	log.Printf("found %d non-fatal errors:\n", len(errors))

	m := map[string]*int{}

	for _, err := range errors {
		if count, found := m[err.Error()]; found {
			(*count)++
		} else {
			var count int = 1
			m[err.Error()] = &count
		}
	}

	keys := make([]string, 0, len(m))

	for key := range m {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		return *m[keys[i]] > *m[keys[j]]
	})

	var s string
	for _, k := range keys {
		s = ""
		if *m[k] != 1 {
			s = "s"
		}
		fmt.Printf("[%d occurrence%s]: %v\n", *m[k], s, k)
	}
}

// loadTimeSeries loads the time series of obsSBE.
//
// Returns nil upon success, otherwise error.
func loadObsSBETimeSeries(
	obsSBE *storagebackends.StorageBackend, features common.StringSet) error {

	nonFatalErrors := []error{}
	err := (*obsSBE).LoadTimeSeries(features, &nonFatalErrors)
	if err != nil {
		return fmt.Errorf("(*obsSBE).LoadTimeSeries() failed: %v", err)
	}

	printNonFatalErrors(nonFatalErrors)

	return nil
}

func main() {

	startTime := time.Now()

	var obsSBE storagebackends.StorageBackend = nil

	// perform cleanup tasks
	cleanup := func() {

		if obsSBE != nil {
			obsSBE.Cleanup()
			obsSBE = nil // ensure only one call to obsSBE.Cleanup()
		}
	}

	// current exit code; may be updated at any point before cleanupAndExit() gets called
	exitCode := 0 // successful/normal termination by default

	// call cleanup() and terminate the program immediately with the current exitCode
	cleanupAndExit := func() {
		cleanup()
		os.Exit(exitCode)
	}

	// ensure cleanupAndExit() gets called upon returning from main()
	defer func() {
		cleanupAndExit()
	}()

	sigs := make(chan os.Signal, 1) // channel that the below signal handler will listen to

	// define termination signals to be passed to sigs
	signal.Notify(
		sigs,
		syscall.SIGHUP,  // kill -SIGHUP <pid> (hangup)
		syscall.SIGINT,  // kill -SIGINT <pid> or Ctrl+C (interrupt)
		syscall.SIGQUIT, // kill -SIGQUIT <pid> (quit)
		syscall.SIGTERM, // kill <pid> (terminated)
	)

	// channel to inform that any of the above termination signals has been caught
	sigCaught := make(chan bool, 1)

	exitOnSignal := true // ensure (to begin with) that a termination signal will result in
	// immediate termination of the program by calling cleanupAndExit()

	// signal handler
	go func() {
		sig := <-sigs // await termination signal
		log.Printf("termination signal caught: %v", sig)
		if exitOnSignal {
			cleanupAndExit()
		}
		sigCaught <- true // send notification
	}()

	// -----------------------------------------------------------------------

	log.Println("serving profiling port...")
	go func() {
		http.ListenAndServe("0.0.0.0:6060", nil)
	}()
	http.DefaultServeMux.Handle("/debug/fgprof", fgprof.Handler())
	go func() {
		log.Println(http.ListenAndServe(":6061", nil)) // TODO: why is log.Println() called here?
	}()

	log.Println("setting readiness to false")
	metaservice.SetReadinessStatus(false)

	if common.Getenv("DUMPREADTOKEN", "") == "true" {
		token, err := auth.CreateReadToken()
		if err != nil {
			log.Printf("auth.CreateReadToken() failed: %v", err)
			exitCode = 1
			return
		}
		fmt.Println(token)
		return
	}

	if common.Getenv("DUMPWRITETOKEN", "") == "true" {
		token, err := auth.CreateWriteToken()
		if err != nil {
			log.Printf("auth.CreateWriteToken() failed: %v", err)
			exitCode = 1
			return
		}
		fmt.Println(token)
		return
	}

	var err error

	err = obs.InitTimeSeriesRegistry()
	if err != nil {
		log.Printf("failed to initialize time series registry: %v", err)
		exitCode = 1
		return
	}

	err = obs.InitRestrictions()
	if err != nil {
		log.Printf("failed to initialize time series restrictions: %v", err)
		exitCode = 1
		return
	}

	eRespItemLimit := common.Getenv("RESPONSEITEMLIMIT", "100000")
	respItemLimit, err := strconv.Atoi(eRespItemLimit)
	if (err != nil) || (respItemLimit <= 0) {
		log.Printf(
			"failed to extract a positive integer from RESPONSEITEMLIMIT (%s)", eRespItemLimit)
		exitCode = 1
		return
	}

	obsSBE, err = obs.CreateStorageBackend(common.Getenv("OBSBACKEND", "local"))
	if err != nil {
		log.Printf("failed to create obs storage backend: %v", err)
		exitCode = 1
		return
	}

	features0 := common.ExtractStringSetFromCSVVals(common.Getenv("FEATURES", ""))
	features := common.StringSet{} // requested features + features that get implicitly added to
	// support other features
	for f := range features0 {
		features.Set(fmt.Sprintf("*%s*", f))
	}

	err = loadObsSBETimeSeries(&obsSBE, features)
	if err != nil {
		log.Printf("loadObsSBETimeSeries() failed: %v", err)
		exitCode = 1
		return
	}

	availableFeatures := common.StringSet{} // all possible features
	activeFeatures := common.StringSet{}    // intersection of features and availableFeatures

	const respWriteTimeout = 90 * time.Second // overall response write timeout

	frostService, err := frost.NewService(
		"./static", respItemLimit, respWriteTimeout, obsSBE, &features, &availableFeatures,
		&activeFeatures)
	if err != nil {
		log.Printf("frost.NewService() failed: %v", err)
		exitCode = 1
		return
	}

	if len(activeFeatures) == 0 {
		fmt.Printf("no features activated; ensure that FEATURES specifies at " +
			"least one of the available features:\n")
		for f := range availableFeatures {
			fmt.Printf("  %s\n", f)
		}
		exitCode = 1
		return
	}

	tsregistry.PrintHeaderIDStats()

	log.Printf("starting Frost server (obs storage backend: %s)", obsSBE.Description())
	metaservice.SetReadinessStatus(true)
	log.Println("setting readiness to true")

	go func() {
		http.ListenAndServe(":8088", frostService.InternalRouter)
	}()

	ctx, cancel := context.WithCancel(context.Background())

	port := common.Getenv("HTTPSERVERPORT", "8080")

	// enable CORS?
	enableCors, _ := strconv.ParseBool(common.Getenv("CORSENABLED", "false"))

	config := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      localhttp.GetHTTPHandler(frostService.ExternalRouter, enableCors),
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
		WriteTimeout: respWriteTimeout,
		IdleTimeout:  10 * time.Second,
		TLSConfig:    config,
	}
	httpServer.RegisterOnShutdown(cancel)

	// run server
	log.Println("start web server...")
	crt := common.Getenv("CRT", "")
	key := common.Getenv("KEY", "")
	if crt != "" && key != "" {
		fmt.Printf("starting up as https, crt: %s key: %s", crt, key)
		go func() {
			if err := httpServer.ListenAndServeTLS(crt, key); err != http.ErrServerClosed {
				log.Printf("httpServer.ListenAndServeTLS() failed: %v", err)
				runtime.Goexit()
			}
		}()
	} else {
		go func() {
			if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
				log.Printf("httpServer.ListenAndServe() failed: %v", err)
				runtime.Goexit()
			}
		}()
	}

	log.Printf(
		"initialization complete in %.2f secs; awaiting signal to terminate program ...",
		time.Since(startTime).Seconds())
	exitOnSignal = false // from this point on we want the signal handler to pass a "signal event"
	// to sigCaught rather than terminating the program immediately by calling cleanupAndExit()
	<-sigCaught // await termination signal
	log.Printf(" ... got one; shutting down ...")

	gracefulCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := httpServer.Shutdown(gracefulCtx); err != nil {
		log.Printf("httpServer.Shutdown failed: %v", err)
		exitCode = 1
		// return // strictly not needed here as we're anyway at the bottom of main()
	} else {
		log.Printf("gracefully stopped")
	}
}
