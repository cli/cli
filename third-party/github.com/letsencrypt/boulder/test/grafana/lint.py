# Check dashboard JSON files for common errors, like forgetting to templatize a
# datasource.
import json
import os
with open(os.path.join(os.path.dirname(os.path.realpath(__file__)),
    "boulderdash.json")) as f:
    dashboard = json.load(f)

# When exporting, the current value of templated variables is saved. We don't
# want to save a specific value for datasource, since that's
# deployment-specific, so we ensure that the dashboard was exported with the
# datasource template variable set to "Default."
for li in dashboard["templating"]["list"]:
    if li["type"] == "datasource":
        assert(li["current"]["value"] == "default")

# Additionally, ensure each panel's datasource is using the template variable
# rather than a hardcoded datasource. Grafana will choose a hardcoded
# datasource on new panels by default, so this is an easy mistake to make.
for ro in dashboard["rows"]:
    for pa in ro["panels"]:
        assert(pa["datasource"] == "$datasource")

# It seems that __inputs is non-empty when template variables at the top of the
# dashboard have been modified from the defaults; check for that.
assert(len(dashboard["__inputs"]) == 0)
