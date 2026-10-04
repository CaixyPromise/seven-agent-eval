"""Helpers for SMALL conformance fixtures; capacity tests must stream parts."""
import json
import urllib.request


def read_fixture(archive):
    run = json.loads(archive.read("run.json"))
    assert run["schema_version"] == "2.0" and run["completed"] <= 1000
    run["cases"] = []
    for name in sorted(archive.namelist()):
        if name.startswith("results/"):
            with archive.open(name) as lines:
                run["cases"].extend(json.loads(line) for line in lines)
    assert len(run["cases"]) == run["completed"]
    return run


def assert_viewer_run(address, expected):
    base = address+"/api/runs/"+expected["id"]
    with urllib.request.urlopen(base, timeout=15) as response:
        actual = json.load(response)
    assert actual == {key:value for key,value in expected.items() if key!="cases"}
    cases = []
    for offset in range(0,expected["completed"],100):
        with urllib.request.urlopen(base+f"/cases?offset={offset}&limit=100", timeout=15) as response:
            page = json.load(response)
        for item in page["items"]:
            from urllib.parse import quote
            with urllib.request.urlopen(base+"/cases/detail?case_id="+quote(item["id"],safe=""),timeout=15) as response:
                cases.append(json.load(response))
    assert cases == expected["cases"]
