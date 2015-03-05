var express = require('express');
var net = require('net');
var app = express();
var server = require('http').createServer(app);
var io = require('socket.io')(server);
var port = process.env.PORT || 3000;

server.listen(port, function () {
  console.log('Server listening at port %d', port);
});

var allowCrossDomain = function(req, res, next) {
  res.header('Access-Control-Allow-Origin', '*');
  res.header('Access-Control-Allow-Methods', 'GET,PUT,POST,DELETE');
  res.header('Access-Control-Allow-Headers', 'Content-Type');

  next();
}

app.use(allowCrossDomain);
app.use(express.static(__dirname + '/public'));

var client_config = {
  host: process.env.PLATFORM_ROUTER_TSP_PORT_877_TCP_ADDR,
  port: process.env.PLATFORM_ROUTER_TSP_PORT_877_TCP_PORT || 877
};

io.on('connection', function(socket) {
  console.log('get socket.io connection, connecting to the tsp router...');

  var client = net.createConnection(client_config, function() {
    console.log('connected to the tsp router...');

    client.on('data', function(data) {
      socket.emit('', data.toString('hex'));
    });

    socket.on('', function(data) {
      client.write(new Buffer(data, 'hex'));
    });
  });

  client.on('error', function(error) {
    console.log('failed to connect to the tsp router... ' + error);
  });
});