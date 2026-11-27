#!/usr/bin/env python3
"""Exercise the public ingress with DSP_RPS=10 and DSP_BURST=2."""
import concurrent.futures
import os
import time
import urllib.error
import urllib.request

base = os.environ['SMOKE_URL']

def request(i):
    req = urllib.request.Request(base + '/source/missing-' + str(i % 3),
                                 data=b'{}', headers={'Host': 'source-' + str(i % 4) + '.test'})
    try:
        response = urllib.request.urlopen(req, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        return response.status, response.read()

started = time.monotonic()
with concurrent.futures.ThreadPoolExecutor(max_workers=32) as pool:
    results = list(pool.map(request, range(200)))
elapsed = time.monotonic() - started
accepted = sum(status == 404 for status, _ in results)
rejected = sum(status == 204 for status, _ in results)
assert accepted > 0 and rejected > 0, (accepted, rejected)
assert accepted <= 3 + elapsed * 10 + 2, (accepted, elapsed)
assert all(status in (204, 404) for status, _ in results), results
assert all(not body for status, body in results if status == 204)
for path in ['/health/ready', '/health/live']:
    with urllib.request.urlopen(base + path, timeout=5) as response:
        assert response.status == 200
# Wait for a new admission, then check the original application contract.
time.sleep(.4)
try:
    urllib.request.urlopen(base + '/source/missing', timeout=5)
    raise AssertionError('Expected method rejection')
except urllib.error.HTTPError as error:
    assert error.code == 405 and error.headers['Allow'] == 'POST'
print(f'PASS: global ingress, accepted={accepted}, rejected={rejected}, elapsed={elapsed:.3f}s; empty 204 and health checks')
