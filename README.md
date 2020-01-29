# myzone.golang
myzone.golang is the core library in the Golang language *for developers of MyZone publishers*. If you just want to run a publisher, then download a binary of that publisher and run it according its instructions. 

myzone is a convention for publishing information on a message bus. This library is part of the reference implementation.
The convention can be found at: https://github.com/hspaay/myzone.convention

## This provides
* systemd launcher for linux 
* Auto publish discovery when nodes and configuration are updated 
* Auto publish updates to output values 
* Management of nodes, inputs and outputs
* Functions for publishing output and discovery messages
* Functions for handling input and configure messages
* MyZone convention data types in Golang

## Getting Started (Linux)

This section describes how to get started building your own MyZone publisher on Linux using this library. The first part describes the project setup to start building. The second part shows how to create a simple weather forecast publisher. The instructions are for Linux but building for MacOs or Windows should not be too different. 

This example uses Go modules as this lets you choose your own project folder location.

## Prerequisites
This guide assumes that you are familiar with programming in golang and golang 1.13 or newer is installed on your development PC. If you are new to golang, check out their website https://golang.org/doc/install for more information. 

A working MQTT broker (server) is needed to test and run a publisher. Mosquitto is a lightweight MQTT broker that runs nicely on Linux (including Raspberry-pi), Mac, and Windows. More info can be found here: https://mosquitto.org/. Only a single broker is needed for all you publishers. For a home automation application you will do fine with running Mosquitto on a Raspberry-pi 2, 3 or 4 connected to a small UPS and park it somewhere out of sight.

Access to git and github.com is needed to retrieve the development libraries. 

A code editor or IDE is needed to edit source code. Visual Studio Code is a free IDE that is quite popular and has good support for golang. See https://code.visualstudio.com/ for more information and downloads.

## Installing

Other than the prerequisites above no other software needs to be installed to start developing a MyZone publisher in golang. 


## Developing Tests

It is highly recommended to use the golang testing facilities. Included in the example are a few basic test cases. To run the tests, the mosquitto broker must be running and reachable on the local network. 

## Deployment (Linux)

The folder structure for deployment as a normal user is:
* ~/bin/myzone/bin      location of the publisher binaries
* ~/bin/myzone/config   location of the configuration files, including myzone.conf
* ~/bin/myzone/logs     logging output

When deploying as an application, create these folders and update /etc/myzone.conf
* /opt/myzone/             location of the publisher binaries
* /etc/myzone,cibf         location of myzone.conf main configuration file
* /var/lib/myzone/         location of the persistence files
* /var/log/myzone/         location of myzone log files

Starting a publisher using systemd 
1. Copy the myzone@.service and myzone.target files to /etc/systemd/system/
2. Edit the paths in myzone.service to make sure the folder and user IDs are correct
3. Start manually using systemd:
   $ sudo service myzone@myweather start
4. To start on bootup:
   $ sudo systemctl enable myzone@myweather

## Contributing
Contribution to the MyZone project is welcome. There are many areas where help is needed, especially with building publishers for IoT and other devices.
See [CONTRIBUTING](docs/CONTRIBUTING.md) for guidelines.

## Questions
For issues and questions, please open a ticket.
Common questions will be captured in the [Q&A](docs/FAQ.md).


# Creating A Publisher 

This example creates a publisher for a weather forecast that updates the forecast every hour. The project folder is *~/Projects/myzone/myweather*

## Step 1: Create A New Project

This uses golang module so you can use the folder of your choice. More info here: https://blog.golang.org/using-go-modules


```bash
$ mkdir -p ~/Projects/myzone/myweather
$ cd ~/Projects/myzone/myweather
$ go mod init myweather
   > go: creating new go.mod: module myweather
```
Create a file named 'myweather.go' that looks like:
```golang
package main
import "github.com/hspaay/myzone.golang"
import "fmt"
func main() {
  fmt.Printf("hello, myweather\n")
}
```
Build and run:
```bash
$ go build
$ ./myweather   
  > Hello, myweather
```

A file go.mod contains the module info include dependencies and versions of the dependencies. 
Make sure go.mod and go.sum are added to your version control.

## Step 2. Initialize The MyZone Publisher
The MyZone publisher is initialized by starting the MyZone running from the library. The runner takes care of starting a process, reading the configuration file, managing nodes,  publishing discovery and output values and reading input values and configuration updates.

Change myweather.go to look like this:
```golang
import "github.com/hspaay/myzone.golang"

class MyWeather extends MyZonePublisher {
}

func main() {
  myWeather = myzone.start(MyWeather)
}
```

# Step 3. Add Nodes
Create a weather forecase node with configuration for latitude, longitude and location name.
The MyZone library will automatically publish the node with any updates including the configuration.

# Step 4. Add Output Values
Obtain the forcast and add it as an output value to the node.
The MyZone library will automatically publish the output discovery and values.





... work in progress ...
