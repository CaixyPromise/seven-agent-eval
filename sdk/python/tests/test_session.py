import pathlib
import tempfile
import unittest
from seven_evaluate.worker import dispatch

class SessionTest(unittest.TestCase):
    def test_sdk_lifecycle_is_explicit(self):
        with tempfile.TemporaryDirectory() as root:
            script=pathlib.Path(root)/"subject.py"
            script.write_text('''from seven_evaluate import subject
def create(ctx): return {"id":"owned"}
def cleanup(ctx): return ctx["session"]["id"]
@subject(id="session", session_create=create, session_cleanup=cleanup)
def invoke(ctx): return {"response":ctx["session"]["id"]}
''')
            params={"path":str(script)}
            self.assertTrue(dispatch("describe",params)["session_lifecycle"])
            session=dispatch("subject/create",params)
            self.assertEqual(dispatch("subject/invoke",{**params,"context":{"session":session}})["response"],"owned")
            self.assertEqual(dispatch("subject/cleanup",{**params,"context":{"session":session}}),"owned")
