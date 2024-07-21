//写一份nodejs实现的web接口.用户访问ping.返回pong
var http = require('http');
var url = require('url');
var server = http.createServer(function(req, res) {
    var pathname = url.parse(req.url).pathname;
    if (pathname == '/ping') {
        res.writeHead(200, {
            'Content-Type': 'text/plain'
        });
        res.end('pong');
    } else {
        res.writeHead(404, {
            'Content-Type': 'text/plain'
        });
        res.end('404 Not Found');
    }
}
);