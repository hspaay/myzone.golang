// Package publisher ...
// - Publishes updates to node, inputs and outputs when they are (re)discovered
// - configuration of nodes
// - control of inputs
// - update of security keys and identity signature
// Thread-safe. All public functions can be invoked from multiple goroutines
package publisher

import (
	"fmt"
	"os"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/iotdomain/iotdomain-go/identities"
	"github.com/iotdomain/iotdomain-go/inputs"
	"github.com/iotdomain/iotdomain-go/lib"
	"github.com/iotdomain/iotdomain-go/messaging"
	"github.com/iotdomain/iotdomain-go/nodes"
	"github.com/iotdomain/iotdomain-go/outputs"
	"github.com/iotdomain/iotdomain-go/types"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

const (
	// DefaultPollInterval in which the registered nodes, inputs and outputs are queried for
	// polling based sources
	DefaultPollInterval = 600

	// RegisteredNodesFileSuffix to append to name of the file containing registered nodes
	RegisteredNodesFileSuffix = "-nodes.json"
	// RegisteredIdentityFileSuffix to append to the name of the file containing publisher saved identity
	RegisteredIdentityFileSuffix = "-identity.json"
	// DomainPublishersFileSuffix to append to the name of the file containing domain publisher identities
	DomainPublishersFileSuffix = "-domainpublishers.json"
	// note, domain nodes are not saved
)

// PublisherConfig defined configuration fields read from the application configuration
type PublisherConfig struct {
	SaveDiscoveredPublishers bool   `yaml:"cachePublishers"`   // load/save discovered publisher identities to cache
	SaveDiscoveredNodes      bool   `yaml:"cacheNodes"`        // load/save discovered nodes to cache
	CacheFolder              string `yaml:"cacheFolder"`       // location of discovered domain nodes and publishers
	ConfigFolder             string `yaml:"configFolder"`      // location of yaml configuration files and registered nodes and identity
	Domain                   string `yaml:"domain"`            // optional override per publisher. Default is local
	PublisherID              string `yaml:"publisherId"`       // this publisher's ID
	Loglevel                 string `yaml:"loglevel"`          // error, warning, info, debug
	Logfile                  string `yaml:"logfile"`           //
	DisableConfig            bool   `yaml:"disableConfig"`     // disable configuration over the bus, default is enabled
	DisableInput             bool   `yaml:"disableInput"`      // disable inputs over the bus, default is enabled
	DisablePublishers        bool   `yaml:"disablePublishers"` // disable listening for available publishers (enable for signature verification)
	SecuredDomain            bool   `yaml:"securedDomain"`     // require secured domain and signed messages
}

// Publisher carries the operating state of 'this' publisher
type Publisher struct {
	config PublisherConfig // determines publisher behavior

	domainIdentities   *identities.DomainPublisherIdentities // discovered publisher identities
	domainInputs       *inputs.DomainInputs                  // discovered inputs from the domain
	domainNodes        *nodes.DomainNodes                    // discovered nodes from the domain
	domainOutputs      *outputs.DomainOutputs                // discovered outputs from the domain
	domainOutputValues *outputs.DomainOutputValues           // output values from the domain

	inputFromHTTP        *inputs.ReceiveFromHTTP        // trigger inputs with http poll result
	inputFromFiles       *inputs.ReceiveFromFiles       // trigger inputs on file changes
	inputFromOutputs     *inputs.ReceiveFromOutputs     // subscribe input to an output (latest) value
	inputFromSetCommands *inputs.ReceiveFromSetCommands // trigger inputs with set commands for registered inputs

	receiveMyIdentityUpdate *identities.ReceiveRegisteredIdentityUpdate
	receiveDomainIdentities *identities.ReceiveDomainPublisherIdentities // listener for identity updates
	receiveNodeConfigure    *nodes.ReceiveNodeConfigure                  // listener for node configure for registered nodes
	receiveSetNodeID        *nodes.ReceiveSetNodeID                      // listener for set node alias

	registeredForecastValues *outputs.RegisteredForecastValues // output forecasts values published by this publisher
	registeredIdentity       *identities.RegisteredIdentity    // registered/published identity of this publisher
	registeredInputs         *inputs.RegisteredInputs          // registered/published inputs from this publisher
	registeredNodes          *nodes.RegisteredNodes            // registered/published nodes from this publisher
	registeredOutputs        *outputs.RegisteredOutputs        // registered/published outputs from this publisher
	registeredOutputValues   *outputs.RegisteredOutputValues   // registered/published output values from this publisher

	// fullIdentity        *types.PublisherFullIdentity                         // this publishers identity
	// identityPrivateKey  *ecdsa.PrivateKey                                    // key for signing and encryption
	isRunning bool // publisher was started and is running
	// runStateAddress string

	messenger           messaging.IMessenger                                 // Message bus messenger to use
	messageSigner       *messaging.MessageSigner                             // publishing signed messages
	onNodeConfigHandler nodes.NodeConfigureHandler                           // handle before applying configuration
	onNodeInputHandler  func(address string, message *types.SetInputMessage) // handle to update device/service input
	pollHandler         func(pub *Publisher)                                 // function that performs value polling
	pollCountdown       int                                                  // countdown each heartbeat
	pollInterval        int                                                  // value polling interval in seconds

	// background publications require a mutex to prevent concurrent access
	heartbeatChannel chan bool
	updateMutex      *sync.Mutex // mutex for async updating and publishing
}

// HandleSetNodeIDCommand handles the command to change the ID of a node. This updates the address
// of a node, its inputs and its outputs.
func (pub *Publisher) HandleSetNodeIDCommand(address string, message *types.SetNodeIDMessage) {
	node := pub.registeredNodes.GetNodeByAddress(address)
	if node == nil {
		return
	}
	pub.registeredNodes.SetNodeID(node, message.NodeID)
	pub.registeredInputs.SetNodeID(node.HWID, message.NodeID)
	pub.registeredOutputs.SetNodeID(node.HWID, message.NodeID)
}

// LoadDomainPublishers loads discovered publisher identities from the cache folder.
// Intended to cache the public signing keys to verify messages from these publishers
func (pub *Publisher) LoadDomainPublishers() error {
	filename := path.Join(pub.config.CacheFolder, pub.PublisherID()+DomainPublishersFileSuffix)
	err := pub.domainIdentities.LoadIdentities(filename)
	return err
}

// LoadRegisteredNodes loads saved registered nodes from the config folder.
// Intended to restore node configuration.
func (pub *Publisher) LoadRegisteredNodes() error {
	filename := path.Join(pub.config.ConfigFolder, pub.PublisherID()+RegisteredNodesFileSuffix)
	err := pub.registeredNodes.LoadNodes(filename)
	return err
}

// SaveDomainPublishers saves discovered domain publisher identities
func (pub *Publisher) SaveDomainPublishers() error {
	filename := path.Join(pub.config.CacheFolder, pub.PublisherID()+DomainPublishersFileSuffix)
	err := pub.domainIdentities.SaveIdentities(filename)
	return err
}

// SaveRegisteredNodes saves current registered nodes to the config folder
func (pub *Publisher) SaveRegisteredNodes() error {
	filename := path.Join(pub.config.ConfigFolder, pub.PublisherID()+RegisteredNodesFileSuffix)
	err := pub.registeredNodes.SaveNodes(filename)
	return err
}

// SetNodeConfigHandler set the handler for updating node configuration.
// The handler is invoked if a configuration update for a node is received and the node exists.
func (pub *Publisher) SetNodeConfigHandler(
	handler func(nodeHWID string, config types.NodeAttrMap)) {

	pub.receiveNodeConfigure.SetConfigureNodeHandler(handler)
}

// SetPollInterval is a convenience function for periodic polling of updates to registered
// nodes, inputs, outputs and output values.
// seconds interval to perform another poll. Default (0) is DefaultPollInterval
// intended for publishers that need to poll for values
func (pub *Publisher) SetPollInterval(seconds int, handler func(pub *Publisher)) {
	logrus.Infof("Publisher.SetPoll: interval = %d seconds", seconds)
	if seconds > 0 {
		pub.pollInterval = seconds
	} else {
		pub.pollInterval = DefaultPollInterval
	}
	pub.pollHandler = handler
}

// SetPublisherStatus sets the publisher runtime status and publishes the message
func (pub *Publisher) SetPublisherStatus(status types.PublisherRunState) {
	addr := identities.MakePublisherStatusAddress(pub.Domain(), pub.PublisherID())
	msg := types.PublisherStatusMessage{
		Address: addr,
		Status:  status,
	}
	identities.PublishStatus(&msg, pub.messageSigner)
}

// Start starts publishing registered nodes, inputs and outputs, and listens for command messages.
// Start will fail if no messenger has been provided.
func (pub *Publisher) Start() {
	logrus.Warningf("Publisher.Start: Starting publisher %s/%s", pub.Domain(), pub.PublisherID())

	if !pub.isRunning {
		pub.updateMutex.Lock()
		pub.isRunning = true
		pub.updateMutex.Unlock()

		go pub.heartbeatLoop()
		// wait for the heartbeat to start
		<-pub.heartbeatChannel

		// reload our own identity and nodes
		myIdent, _ := pub.registeredIdentity.GetFullIdentity()
		pub.domainIdentities.AddIdentity(&myIdent.PublisherIdentityMessage)

		// reload previously discovered publishers
		if pub.config.SaveDiscoveredPublishers {
			pub.domainIdentities.LoadIdentities(pub.config.CacheFolder)
		}
		// reload previously discovered nodes
		if pub.config.SaveDiscoveredNodes {
			pub.domainNodes.LoadNodes(pub.config.CacheFolder)
		}

		// discover domain entities, eg identities, nodes, inputs and outputs
		if !pub.config.DisablePublishers {
			pub.receiveDomainIdentities.Start()
		}
		// receive registered input set commands
		if !pub.config.DisableInput {
			pub.receiveSetNodeID.Start()
		}
		// Receive registered node configuration commands
		if !pub.config.DisableConfig {
			pub.receiveNodeConfigure.Start()
		}
		// in secured domains the DSS can update the identity
		if pub.config.SecuredDomain {
			pub.receiveMyIdentityUpdate.Start()
		}
		//  listening
		lwtStatusAddress := identities.MakePublisherStatusAddress(pub.Domain(), pub.PublisherID())
		pub.messenger.Connect(lwtStatusAddress, string(types.PublisherRunStateLost))

		pub.SetPublisherStatus(types.PublisherRunStateConnected)
		identities.PublishIdentity(&myIdent.PublisherIdentityMessage, pub.messageSigner)
	}
}

// Stop publishing, and set the run status to disconnected and disconnect from the
// message bus. Wait until the heartbeat loop has finished processing messages
func (pub *Publisher) Stop() {
	logrus.Warningf("Publisher.Stop: Stopping publisher %s", pub.PublisherID())
	pub.updateMutex.Lock()
	if pub.isRunning {
		pub.isRunning = false

		pub.receiveMyIdentityUpdate.Stop()
		pub.receiveDomainIdentities.Stop()
		pub.receiveNodeConfigure.Stop()
		pub.receiveSetNodeID.Stop()

		pub.updateMutex.Unlock()
		// wait for heartbeat to end
		<-pub.heartbeatChannel
	} else {
		pub.updateMutex.Unlock()
	}
	pub.SetPublisherStatus(types.PublisherRunStateDisconnected)
	pub.messenger.Disconnect()
	logrus.Info("... bye bye")
}

// WaitForSignal waits until a TERM or INT signal is received
func (pub *Publisher) WaitForSignal() {

	// catch all signals since not explicitly listing
	exitChannel := make(chan os.Signal, 1)

	//signal.Notify(exitChannel, syscall.SIGTERM|syscall.SIGHUP|syscall.SIGINT)
	signal.Notify(exitChannel, syscall.SIGINT, syscall.SIGTERM)

	sig := <-exitChannel
	log.Warningf("RECEIVED SIGNAL: %s", sig)
	fmt.Println()
	fmt.Println(sig)
}

// Main heartbeat loop to publish, discove and poll value updates
func (pub *Publisher) heartbeatLoop() {
	logrus.Infof("Publisher.heartbeatLoop: starting heartbeat loop")
	pub.heartbeatChannel <- false

	for {
		time.Sleep(time.Second)

		// FIXME: The duration of publishing these updates adds to the heartbeat which delays the heartbeat
		pub.PublishUpdates()

		if pub.config.SaveDiscoveredPublishers && pub.domainIdentities.UpdateCount() > 0 {
			pub.SaveDomainPublishers()
		}

		// poll for discovery and values of registered nodes, inputs and outputs
		if (pub.pollCountdown <= 0) && (pub.pollHandler != nil) {
			pub.pollHandler(pub)
			pub.pollCountdown = pub.pollInterval
		}
		pub.pollCountdown--

		pub.updateMutex.Lock()
		isRunning := pub.isRunning
		pub.updateMutex.Unlock()
		if !isRunning {
			break
		}
	}
	pub.heartbeatChannel <- true
	logrus.Infof("Publisher.heartbeatLoop: Ending loop of publisher %s", pub.PublisherID())
}

// SetLogging sets the logging level and output file for this publisher
// Intended for setting logging from configuration
//  levelName is the requested logging level: error, warning, info, debug
//  filename is the output log file full name including path, use "" for stderr
func SetLogging(levelName string, filename string) error {
	loggingLevel := log.DebugLevel
	var err error

	if levelName != "" {
		switch strings.ToLower(levelName) {
		case "error":
			loggingLevel = log.ErrorLevel
		case "warn":
		case "warning":
			loggingLevel = log.WarnLevel
		case "info":
			loggingLevel = log.InfoLevel
		case "debug":
			loggingLevel = log.DebugLevel
		}
	}
	logOut := os.Stderr
	if filename != "" {
		logFileHandle, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
		if err != nil {
			err = lib.MakeErrorf("Publisher.SetLogging: Unable to open logfile: %s", err)
		} else {
			logrus.Warnf("Publisher.SetLogging: Send logging output to %s", filename)
			logOut = logFileHandle
		}
	}

	logrus.SetFormatter(
		&log.TextFormatter{
			// LogFormat: "",
			// DisableColors:   true,
			// DisableLevelTruncation: true,
			// PadLevelText:    true,
			TimestampFormat: "2006-01-02 15:04:05.000",
			FullTimestamp:   true,
			// ForceFormatting: true,
		})
	logrus.SetOutput(logOut)
	logrus.SetLevel(loggingLevel)

	logrus.SetReportCaller(false) // publisher logging includes caller and file:line#
	return err
}

// NewPublisher creates a new publisher instance. This is used for all publications.
//
// The configFolder contains the publisher saved identity and node configuration <publisherID>-nodes.json.
// which is loaded during Start(). Use "" for default config folder. When autosave is set then the configuration
// files are written when identity or registered nodes update.
//  domain and publisherID identify this publisher. If the identity file does not match these, it
// is discarded and a new identity is created. If the publisher has joined the domain and the DSS has issued
// the identity then changing domain or publisherID invalidates the publisher and it has to rejoin
// the domain. If no domain is provided, the default 'local' is used.
//
// signingMethod indicates if and how publications must be signed. The default is jws. For testing 'none' can be used.
//
// messenger for publishing onto the message bus is required
func NewPublisher(config *PublisherConfig, messenger messaging.IMessenger,
) *Publisher {

	if messenger == nil {
		return nil
	}
	if config == nil {
		config = &PublisherConfig{}
	}
	if config.Domain == "" {
		config.Domain = types.LocalDomainID
	}
	if config.ConfigFolder == "" {
		config.ConfigFolder = lib.DefaultConfigFolder
	}
	SetLogging(config.Loglevel, config.Logfile)

	identityFile := path.Join(config.ConfigFolder, config.PublisherID+RegisteredIdentityFileSuffix)
	registeredIdentity := identities.NewRegisteredIdentity(
		config.Domain, config.PublisherID, identityFile)
	_, privKey, err := registeredIdentity.LoadIdentity()
	if err != nil {
		// save the identity as the loaded one isnt' valid
		registeredIdentity.SaveIdentity()
	}
	domainIdentities := identities.NewDomainPublisherIdentities()

	// These are the basis for signing and identifying publishers
	messageSigner := messaging.NewMessageSigner(messenger, privKey, domainIdentities.GetPublisherKey)

	// application services
	domainInputs := inputs.NewDomainInputs(messageSigner)
	domainNodes := nodes.NewDomainNodes(messageSigner)
	domainOutputs := outputs.NewDomainOutputs(messageSigner)
	domainOutputValues := outputs.NewDomainOutputValues(messageSigner)
	registeredInputs := inputs.NewRegisteredInputs(config.Domain, config.PublisherID)
	registeredNodes := nodes.NewRegisteredNodes(config.Domain, config.PublisherID)
	registeredOutputs := outputs.NewRegisteredOutputs(config.Domain, config.PublisherID)
	registeredOutputValues := outputs.NewRegisteredOutputValues(config.Domain, config.PublisherID)
	registeredForecastValues := outputs.NewRegisteredForecastValues(config.Domain, config.PublisherID)

	receiveMyIdentityUpdate := identities.NewReceiveRegisteredIdentityUpdate(
		registeredIdentity, messageSigner)
	receiveDomainIdentities := identities.NewReceivePublisherIdentities(config.Domain,
		domainIdentities, messageSigner)
	receiveNodeConfigure := nodes.NewReceiveNodeConfigure(
		config.Domain, config.PublisherID, nil, messageSigner, registeredNodes, privKey)
	receiveSetNodeID := nodes.NewReceiveSetNodeID(
		config.Domain, config.PublisherID, nil, messageSigner, privKey)

	var pub = &Publisher{
		config:             *config,
		domainIdentities:   domainIdentities,
		domainInputs:       domainInputs,
		domainNodes:        domainNodes,
		domainOutputs:      domainOutputs,
		domainOutputValues: domainOutputValues,

		inputFromSetCommands: inputs.NewReceiveFromSetCommands(
			config.Domain, config.PublisherID, messageSigner, registeredInputs),
		inputFromHTTP:    inputs.NewReceiveFromHTTP(registeredInputs),
		inputFromFiles:   inputs.NewReceiveFromFiles(registeredInputs),
		inputFromOutputs: inputs.NewReceiveFromOutputs(messageSigner, registeredInputs),

		heartbeatChannel: make(chan bool),
		// fullIdentity:       identity,
		// identityPrivateKey: privateKey,
		// runStateAddress: fmt.Sprintf("%s/%s/%s", config.Domain, config.PublisherID, types.MessageTypeRunState),

		messenger:               messenger,
		messageSigner:           messageSigner,
		pollCountdown:           0,
		pollInterval:            DefaultPollInterval,
		receiveDomainIdentities: receiveDomainIdentities,
		receiveMyIdentityUpdate: receiveMyIdentityUpdate,
		receiveNodeConfigure:    receiveNodeConfigure,
		receiveSetNodeID:        receiveSetNodeID,

		registeredForecastValues: registeredForecastValues,
		registeredIdentity:       registeredIdentity,
		registeredInputs:         registeredInputs,
		registeredNodes:          registeredNodes,
		registeredOutputs:        registeredOutputs,
		registeredOutputValues:   registeredOutputValues,

		updateMutex: &sync.Mutex{},
	}
	receiveSetNodeID.SetNodeIDHandler(pub.HandleSetNodeIDCommand)

	// Load configuration of previously registered nodes from config
	pub.LoadRegisteredNodes()

	return pub
}
