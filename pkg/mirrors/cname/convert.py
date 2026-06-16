import json
import urllib.request

URL = 'https://raw.githubusercontent.com/mirrorz-org/mirrorz/refs/heads/master/static/json/cname.json'

with urllib.request.urlopen(URL) as f:
    data = json.load(f)

# Invert
inverted = {}
for alias, cname in data.items():
    inverted.setdefault(cname, []).append(alias)

# Sort keys
with open('pkg/mirrors/cname/cname.yaml', 'w') as f:
    f.write('# cname: [ alias1, alias2, ...]\n')
    for cname in sorted(inverted.keys(), key=str.lower):
        aliases = sorted(inverted[cname])
        f.write(f'{cname}: [{", ".join(aliases)}]\n')

print('lines written:', len(inverted))
