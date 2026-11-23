#!/usr/bin/env python3
"""Exercise a disposable local installation through its public HTTP interface."""
import http.cookiejar
import json
import os
import re
import sys
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request

base = os.environ.get('SMOKE_URL', 'http://127.0.0.1:18080')
client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))

def request(path, form=None, payload=None):
    data = None
    headers = {}
    if form is not None:
        data = urllib.parse.urlencode(form, doseq=True).encode()
    if payload is not None:
        data = json.dumps(payload).encode()
        headers['Content-Type'] = 'application/json'
    with client.open(urllib.request.Request(base + path, data=data, headers=headers), timeout=5) as r:
        return r.status, r.read().decode()

def csrf(html):
    return re.search(r'name="csrf" value="([a-f0-9]+)"', html)[1]

def wait_revision():
    for _ in range(50):
        _, html = request('/status')
        numbers = re.findall(r'<strong>(\d+)</strong>', html)
        if len(numbers) == 2 and numbers[0] == numbers[1] and 'DSP ready' in html:
            return
        time.sleep(.1)
    raise AssertionError('configuration was not applied/ready')

_, html = request('/login')
_, html = request('/login', {'csrf': csrf(html), 'login': 'smoke', 'password': os.environ['ADMIN_PASSWORD']})
assert 'Create traffic source' in html, 'login failed'
token = csrf(html)
if '--verify' in sys.argv:
    assert 'Smoke source' in html, 'source did not survive recreation'
    _, html = request('/campaigns')
    assert 'Smoke campaign' in html and re.search(r'\bStopped\b', html, re.I), 'campaign did not survive recreation'
    campaign = re.search(r'/campaign\?id=([a-f0-9]+)">Smoke campaign</a>', html)[1]
    _, edit_html = request('/campaign?id=' + campaign)
    assert '81.2.69.160' in edit_html and 'id="ip_enabled" checked' in edit_html, 'IP settings did not survive recreation'
    assert '445566\nsite-A' in edit_html and 'id="placement_enabled" checked' in edit_html, 'placement settings lost'
    assert '{placement_id}' in edit_html, 'URL template lost'
    wait_revision()
    print('PASS: login, source, stopped campaign, IP settings and revision survived container recreation')
    sys.exit(0)

_, html = request('/sources', {'csrf': token, 'name': 'Smoke source', 'adapter': 'exads_json_pop'})
source = re.search(r'/source/([a-f0-9]+)', html)[1]
form = {'csrf': token, 'name': 'Smoke campaign', 'url': 'https://example.com', 'cpm': '0.13',
        'active': 'on', 'countries': 'GB', 'platforms': 'desktop', 'browsers': 'Chrome', 'browser_majors': '66'}
_, html = request('/campaign', form)
campaign = re.search(r'/campaign\?id=([a-f0-9]+)', html)[1]
wait_revision()
payload = {'id': 'smoke-request', 'type': 'popunder', 'sub': 445566, 'ip': '81.2.69.160',
           'ua': 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/66.0.3359.139 Safari/537.36'}
status, body = request('/source/' + source, payload=payload)
assert status == 200 and json.loads(body)['bid'] == {'id': 'smoke-request', 'url': 'https://example.com', 'btype': 1, 'value': .13}, body
if os.environ.get('LOADCHECK') == '1':
    for unique in ('false', 'true'):
        subprocess.run(['go', 'run', './cmd/loadtest', '-url', base + '/source/' + source,
                        '-duration', '5s', '-rps', '10000', '-unique-ua=' + unique], check=True)
# A matching narrower campaign wins even at the lower CPM.
form.update({'id': campaign, 'cpm': '1.2', 'ip_enabled': 'on', 'ip_entries': '81.2.69.159/24'})
request('/campaign', form)
_, html = request('/campaign', {'csrf': token, 'name': 'All competitor', 'url': 'https://example.com/all', 'cpm': '1.3', 'active': 'on'})
all_id = re.search(r'/campaign\?id=([a-f0-9]+)">All competitor</a>', html)[1]
wait_revision()
def winning_url():
    status, body = request('/source/' + source, payload=payload)
    assert status == 200, (status, body)
    return json.loads(body)['bid']['url']
assert winning_url() == 'https://example.com', 'matched IP 1.2 must beat All 1.3'
form.update({'cpm': '2', 'ip_entries': '10.0.0.0/8'})
request('/campaign', form)
wait_revision()
assert winning_url() == 'https://example.com/all', 'nonmatching IP campaign admitted'
form.pop('ip_enabled')
request('/campaign', form)
wait_revision()
assert winning_url() == 'https://example.com', 'disabled IP filter is not All'
_, html = request('/campaign?id=' + campaign)
assert '10.0.0.0/8' in html, 'disabled list was erased'
form['ip_enabled'] = 'on'
request('/campaign', form)
wait_revision()
assert winning_url() == 'https://example.com/all', 're-enabled filter did not apply'
_, status_before = request('/status')
before = re.findall(r'<strong>(\d+)</strong>', status_before)
try:
    request('/campaign', dict(form, ip_entries='81.2.69.160\n\nbad-entry'))
    raise AssertionError('invalid IP list accepted')
except urllib.error.HTTPError as error:
    invalid_html = error.read().decode()
    assert error.code == 400 and 'line 3' in invalid_html and 'bad-entry' in invalid_html
_, status_after = request('/status')
assert before == re.findall(r'<strong>(\d+)</strong>', status_after), 'invalid list changed revision'
form['ip_entries'] = '81.2.69.160'
request('/campaign', form)
wait_revision()
assert winning_url() == 'https://example.com', 'exact IP did not match'
request('/campaign', {'csrf': token, 'id': all_id, 'name': 'All competitor', 'url': 'https://example.com/all', 'cpm': '1.3', 'active': 'off'})
print('PASS: CIDR vs All auction, mismatched IP, disable/preserve/re-enable, exact IP, invalid form without revision')
# Placement filter and tracker macro, with the IP filter still active.
form.update({'placement_enabled': 'on', 'placement_mode': 'whitelist',
             'placement_entries': '445566\nsite-A', 'url': 'https://example.com/?site={placement_id}'})
request('/campaign', form)
wait_revision()
assert winning_url() == 'https://example.com/?site=445566'
payload['sub'] = 'other'
assert request('/source/' + source, payload=payload) == (204, '')
form['placement_mode'] = 'blacklist'
request('/campaign', form)
wait_revision()
assert winning_url() == 'https://example.com/?site=other'
payload['sub'] = 445566
assert request('/source/' + source, payload=payload) == (204, '')
form.pop('placement_enabled')
request('/campaign', form)
wait_revision()
payload['sub'] = 'site& +#%'
assert winning_url() == 'https://example.com/?site=site%26+%2B%23%25'
_, edit_html = request('/campaign?id=' + campaign)
assert '445566\nsite-A' in edit_html, 'disabled placement list lost'
for bad_sub in (None, '', '   '):
    payload['sub'] = bad_sub
    try:
        request('/source/' + source, payload=payload)
        raise AssertionError('invalid placement accepted')
    except urllib.error.HTTPError as error:
        assert error.code == 400
payload['sub'] = 445566
form['placement_enabled'] = 'on'
form['placement_mode'] = 'whitelist'
print('PASS: placement WL/BL, disabled list retained, URL macro escaped, invalid placement -> 400')
form.update({'id': campaign, 'active': 'off'})
request('/campaign', form)
wait_revision()
status, body = request('/source/' + source, payload=payload)
assert status == 204 and body == '', (status, body)
try:
    request('/internal/config/status')
    raise AssertionError('internal API exposed publicly')
except urllib.error.HTTPError as e:
    assert e.code == 404
print('PASS: login -> source -> campaign -> real MMDB/UA targeting -> EXADS bid -> stop -> 204; internal API is private')
