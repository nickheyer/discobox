#!/bin/bash

# Start 3 test backend servers on ports 8991, 8992, 8993

echo "Starting test backend on port 8991..."
python3 -c "
from http.server import HTTPServer, BaseHTTPRequestHandler

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'you are on port 8991\\n')
    
    def log_message(self, format, *args):
        print(f'[8991] {format % args}')

HTTPServer(('', 8991), Handler).serve_forever()
" &

echo "Starting test backend on port 8992..."
python3 -c "
from http.server import HTTPServer, BaseHTTPRequestHandler

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'you are on port 8992\\n')
    
    def log_message(self, format, *args):
        print(f'[8992] {format % args}')

HTTPServer(('', 8992), Handler).serve_forever()
" &

echo "Starting test backend on port 8993..."
python3 -c "
from http.server import HTTPServer, BaseHTTPRequestHandler

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header('Content-type', 'text/plain')
        self.end_headers()
        self.wfile.write(b'you are on port 8993\\n')
    
    def log_message(self, format, *args):
        print(f'[8993] {format % args}')

HTTPServer(('', 8993), Handler).serve_forever()
" &

echo "All test backends started. Press Ctrl+C to stop all."
wait