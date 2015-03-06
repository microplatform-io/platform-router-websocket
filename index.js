var express = require('express');
var net = require('net');
var app = express();
var server = require('http').createServer(app);
var io = require('socket.io')(server);
var port = process.env.PORT || 3000;

String.prototype.lpad = function(padString, length) {
  var str = this;
  while (str.length < length)
    str = padString + str;
  return str;
}

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
}

var request_is_complete = function(request) {
  return request.packets.length == request.packets[0].total_sequences;
}

var array_buffer_to_hex = function(array_buffer) {
  var uint8_array = new Uint8Array(array_buffer), 
    hex = '';

  for(var i = 0; i < uint8_array.byteLength; i++) {
    hex += uint8_array[i].toString(16).lpad('0', 2);
  }

  return hex;
}

var generate_request_from_packets = function(packets) {
  return {
    request_id: packets[0].request_id,
    method: packets[0].method,
    resource: packets[0].resource,
    payload: get_payload_from_packets(packets)
  }
}

var get_payload_from_packets = function(packets) {
  var payload = '';
  for(var i = 0; i < packets.length; i++) {
    payload += packets[i].payload;
  }
  return payload;
}

io.on('connection', function(socket) {
  console.log('get socket.io connection, connecting to the tsp router...');

  var requests = {},
      pending_packet = null;

  var client = net.createConnection(client_config);

  client.on('connect', function() {
    console.log('connected to the tsp router...');
  });

  client.on('error', function(error) {
    console.log('failed to connect to the tsp router... ' + error);
  });

  client.on('data', function(data) {
    data = data.toString('hex');

    var offset = 0;

    while(offset < data.length) {
      var packet = null;

      if(pending_packet == null) {
        var header_bytes = data.slice(offset, offset+56);
        offset += 56;

        packet = {
          method: parseInt(header_bytes.slice(3,4), 16),
          resource: parseInt(header_bytes.slice(4,8), 16),
          sequence: parseInt(header_bytes.slice(8,12), 16),
          total_sequences: parseInt(header_bytes.slice(12,16), 16),
          request_id: header_bytes.slice(16, 48),
          payload_length: parseInt(header_bytes.slice(48, 56), 16),
          payload: ''
        }
      } else {
        packet = pending_packet;
      }

      var payload = data.slice(offset, (offset + packet.payload_length * 2) - packet.payload.length);
      offset += payload.length;

      packet.payload += payload;

      if(packet.payload.length == packet.payload_length * 2) {
        pending_packet = null;
        requests[packet.request_id].packets.push(packet);

        var request = requests[packet.request_id];
        if(request_is_complete(request)) {
          socket.emit(packet.request_id, generate_request_from_packets(request.packets));

          delete requests[packet.request_id];
        }
      } else {
        pending_packet = packet;
      }
    }
  });

  socket.on('request', function(request) {
    requests[request.request_id] = {packets:[]};

    var data = '020' + request.method.toString(16) + request.resource.toString(16).lpad("0", 4);
    data += (0).toString(16).lpad("0", 4) + (1).toString(16).lpad("0", 4);
    data += request.request_id;
    data += request.protobuf.length.toString(16).lpad("0", 8);
    data += array_buffer_to_hex(request.protobuf);

    client.write(new Buffer(data, 'hex'));
  });
});