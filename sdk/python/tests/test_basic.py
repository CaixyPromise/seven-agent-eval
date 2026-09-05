import json
import pathlib
import unittest
from jsonschema import Draft202012Validator
from seven_evaluate import basic

ROOT = pathlib.Path(__file__).resolve().parents[3]
class ContractTests(unittest.TestCase):
    def test_three_ingress_shapes(self):
        examples = ROOT / "protocol/examples"
        active = json.loads((examples/"active.json").read_text())["value"]
        replay = json.loads((examples/"replay.json").read_text())["value"]
        event = json.loads((examples/"production.json").read_text())["value"]
        def observations(case, result):
            return basic.evaluate({"case": case, "result": result, "status": "completed"})
        self.assertEqual(observations(replay, active), observations(event["case"], event["result"]))
        self.assertEqual(observations(replay, active), observations(replay, {"response": replay["response"], "contexts": replay["contexts"]}))
    def test_schema_examples_and_invalid(self):
        schema = json.loads((ROOT/"protocol/schemas/exchange.json").read_text())
        validator = Draft202012Validator(schema)
        for path in (ROOT/"protocol/examples").glob("*.json"):
            validator.validate(json.loads(path.read_text()))
        self.assertFalse(validator.is_valid({"kind":"case","value":{"inputs":"x"}}))
        invalid = basic.metric("a", "output.present", "error", 0)
        self.assertFalse(validator.is_valid({"kind":"metric","value":invalid}))
    def test_missing_is_not_zero(self):
        out = basic.evaluate({"case":{"id":"x","inputs":"x"},"result":{},"status":"error"})
        self.assertEqual(out[0]["status"], "not_computable")
        self.assertIsNone(out[0]["value"])
    def test_false_is_observed_and_exact_opt_in(self):
        out = basic.evaluate({"case":{"id":"x","inputs":"x"},"result":{"response":""},"status":"completed"})
        self.assertFalse(out[0]["value"])
        self.assertEqual(out[0]["status"], "observed")
        self.assertEqual(out[2]["status"], "not_applicable")
    def test_statistical_identity(self):
        metrics = [basic.metric("a", "recall", "observed", .2, parameters={"k":5}),
                   basic.metric("a", "recall", "observed", .8, parameters={"k":10})]
        out = basic.process({"total":2,"input_digest":"d","cases":[{"case":{"id":"a"},"status":"completed","metrics":metrics}]})
        self.assertEqual(len(out["data"]["series"]), 2)
        self.assertEqual(out["data"]["coverage"], .5)
    def test_replaced_processor_has_no_stale_conclusion(self):
        self.assertEqual(basic.conclude({"outputs":[]})[0]["code"],"dependency_missing")

    def test_exact_keeps_json_boolean_and_number_distinct(self):
        for response, reference in [(False, 0), ({"a": True}, {"a": 1}), ([False], [0])]:
            out = basic.evaluate({"case":{"id":"x","inputs":"x","references":{"exact":reference}},
                                  "result":{"response":response},"status":"completed"})
            self.assertFalse(next(m for m in out if m["metric_id"] == "output.exact")["value"])

if __name__ == "__main__":
    unittest.main()
