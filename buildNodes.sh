protoc --go_out=. ./proto/*.proto
go build .
cp poh-client.com ./builds/node1/ 
cp poh-client.com ./builds/node2/ 
cp poh-client.com ./builds/node3/ 
cp poh-client.com ./builds/node4/