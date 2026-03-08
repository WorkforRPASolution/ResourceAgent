#!/usr/bin/env python3
"""
Mock ServiceDiscovery HTTP server for ResourceAgent local E2E testing.

Returns a static JSON response matching the real ServiceDiscovery /EARS/Service/Multi endpoint.
Usage:
    python3 mock_servicediscovery.py [--port PORT] [--kafkarest ADDR]

Default: port=50009, kafkarest=127.0.0.1:8082
"""

import argparse
import json
import signal
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler


class ServiceDiscoveryHandler(BaseHTTPRequestHandler):
    services = {}

    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        self.wfile.write(json.dumps(self.services).encode())

    def log_message(self, format, *args):
        # Suppress request logs in test mode; override with -v if needed
        pass


def main():
    parser = argparse.ArgumentParser(description="Mock ServiceDiscovery for E2E testing")
    parser.add_argument("--port", type=int, default=50009, help="Listen port (default: 50009)")
    parser.add_argument("--kafkarest", default="127.0.0.1:8082", help="KafkaRest address to return")
    args = parser.parse_args()

    ServiceDiscoveryHandler.services = {"KafkaRest": args.kafkarest}

    server = HTTPServer(("127.0.0.1", args.port), ServiceDiscoveryHandler)

    def shutdown(signum, frame):
        server.shutdown()
        sys.exit(0)

    signal.signal(signal.SIGTERM, shutdown)
    signal.signal(signal.SIGINT, shutdown)

    print(f"Mock ServiceDiscovery listening on 127.0.0.1:{args.port}")
    print(f"  KafkaRest -> {args.kafkarest}")
    server.serve_forever()


if __name__ == "__main__":
    main()
