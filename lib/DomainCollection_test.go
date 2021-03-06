package lib_test

import (
	"reflect"
	"testing"

	"github.com/iotdomain/iotdomain-go/lib"
	"github.com/iotdomain/iotdomain-go/messaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ItemType struct {
	Address     string
	PublisherID string
	Name        string
}

func TestCreateCollection(t *testing.T) {
	item1 := ItemType{Name: "Hello"}
	item1Addr := "domain/pub/node"
	item2 := ItemType{Name: "World"}
	item2NodeBase := "domain/pub/node2"
	item2InputBase := item2NodeBase + "/type/instance"
	item2InputAddr := item2InputBase + "/$input"
	config := messaging.MessengerConfig{}
	msg := messaging.NewDummyMessenger(&config)
	signer := messaging.NewMessageSigner(msg, nil, nil)

	c := lib.NewDomainCollection(reflect.TypeOf(&item1), signer.GetPublicKey)
	require.NotNil(t, c)
	c.Update(item1Addr, &item1)
	c.Update(item2InputAddr, &item2)

	// test getby address
	item1b := c.GetByAddress(item1Addr)
	require.NotNil(t, item1b, "Nil result on GetByAddress ")
	item1b = c.GetByAddress(item1Addr + "/$set")
	require.NotNil(t, item1b, "Nil result on GetByAddress ")

	// test get node
	item1b = c.Get(item1Addr, "", "")
	require.NotNil(t, item1b, "Nil result on GetByAddress ")
	item1b = c.Get("domain", "", "")
	require.Nil(t, item1b, "expected nil")

	// get all
	typedList := make([]*ItemType, 0)
	c.GetAll(&typedList)
	assert.Equal(t, 2, len(typedList))

	// test getting inputs
	item2b := c.GetByAddress(item2InputBase)
	require.NotNil(t, item2b, "Nil result on GetByAddress ")
	item2b = c.Get(item2NodeBase, "type", "instance")
	require.NotNil(t, item2b, "Nil result on Get ")
	item2b = c.Get(item2InputBase, "not", "found")
	require.Nil(t, item2b, "Not nil result for invalid address ")
	itemList := make([]*ItemType, 0)
	c.GetByAddressPrefix(item2NodeBase, &itemList)
	assert.Equal(t, 1, len(itemList), "Unexpected nr items of node")

	// cleanup
	c.Remove(item1Addr)
	item1b = c.GetByAddress(item1Addr)
	require.Nil(t, item1b, "Item still there after remove")
}
func TestDiscovery(t *testing.T) {
	const itemAddr = "test/pub1/node1/type/instance"
	errCount := 0
	privKey := messaging.CreateAsymKeys()
	item1 := ItemType{Address: itemAddr, PublisherID: "Hello", Name: "World"}
	config := messaging.MessengerConfig{}
	msg := messaging.NewDummyMessenger(&config)
	signer := messaging.NewMessageSigner(msg, privKey, nil)

	c := lib.NewDomainCollection(reflect.TypeOf(&ItemType{}), signer.GetPublicKey)
	require.NotNil(t, c)
	signer.Subscribe("test/+/#", func(addr string, msg string) error {
		err := c.HandleDiscovery(addr, msg, &ItemType{})
		if err != nil {
			errCount++
		}
		return err
	})

	err := signer.PublishObject(itemAddr, false, &item1, nil)
	require.NoError(t, err)
	discoObject := c.GetByAddress(itemAddr)
	require.NotNil(t, discoObject, "published object not found")
	// must be of ItemType
	discoItem := discoObject.(*ItemType)
	assert.Equal(t, "World", discoItem.Name, "No name")

	err = signer.PublishObject(itemAddr, false, "bad object", nil)
	assert.Equal(t, 1, errCount, "Bad message not received")

	item1c := c.GetByAddress(itemAddr)
	require.NotNil(t, item1c)
}

func TestMakeError(t *testing.T) {
	testError := lib.MakeErrorf("This is a test error")
	assert.Error(t, testError, "Expected to see an error")
}
