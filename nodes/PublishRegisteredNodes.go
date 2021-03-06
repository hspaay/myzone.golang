package nodes

import (
	"github.com/iotdomain/iotdomain-go/messaging"
	"github.com/iotdomain/iotdomain-go/types"
	"github.com/sirupsen/logrus"
)

// PublishRegisteredNodes publishes pending updates to registered nodes and saves their configuration to file
// the node configuration is saved in file <publisherID>-nodes.yaml
func PublishRegisteredNodes(
	updatedNodes []*types.NodeDiscoveryMessage,
	messageSigner *messaging.MessageSigner) {

	// updatedNodes := regNodes.GetUpdatedNodes(true)

	// publish updated nodes
	for _, node := range updatedNodes {
		if node != nil {
			logrus.Infof("PublishRegisteredNodes: publish node discovery: %s", node.Address)
			messageSigner.PublishObject(node.Address, true, node, nil)
		} else {
			// node was deleted
			// TODO: remove node from the message bus
		}
	}
}
