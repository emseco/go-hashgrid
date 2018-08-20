# go-hashgrid

Go implementation of the Extrēmus. 

Extrēmus is the first public blockchain platform based on the combination of Jigsaw-puzzle DAG and Sharding, with extremely high scalability, perfect consistency and excellent security property. The Jigsaw-puzzle DAG model proposed by Extrēmus is a perfect solution for the global consistency problem in DAG. http://www.emseco.io.

## Building And Run go-hashgrid Node

1. Install golang

2. Set %GOPATH% Environment Variable 

3. Download go-hashgrid code to GOPATH/src

    cd %GOPATH%/src
    git clone https://github.com/emseco/go-hashgrid.git

4. Run test/test.bat

	cd test
    test.bat

5. Run main.exe

	main.exe
	
6. The config file. 

    example：
    { 
	"addr":"127.0.0.1:33201", //local node address
	"coinbase":[10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,33],//local node public key
	"peers":["127.0.0.1:33201","127.0.0.1:33202","127.0.0.1:33203"],//seed nodes
	"interMilli":500//max time(millisecond) of generate block
	}
	
