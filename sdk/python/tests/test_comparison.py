import unittest
from seven_evaluate.basic import process


class ComparisonTest(unittest.TestCase):
    def ctx(self):
        def case(candidate, cid, value, currency="USD"):
            return {"case":{"id":candidate+"/"+cid,"metadata":{"seven_eval_candidate":candidate,"seven_eval_case_id":cid}},
                    "status":"completed","result":{"parameter_receipt":{"state":"unconfirmed"}},
                    "metrics":[{"metric_id":"rag.f1","evaluator":"rag@1","parameters":{"k":5},"status":"not_computable" if value is None else "observed","value":value},
                               {"metric_id":"usage.cost","evaluator":"basic@1","parameters":{"currency":currency,"source":"measured"},"status":"observed","value":1}]}
        return {"total":4,"input_digest":"results", "comparison":{"baseline_id":"base","case_ids":["q1","q2"],"input_digest":"frozen"},
                "candidates":[{"id":"base","target_parameters":{"top_k":1}},{"id":"new","target_parameters":{"top_k":3}}],
                "cases":[case("base","q1",0),case("base","q2",1),case("new","q1",.2),case("new","q2",None,"CNY")]}

    def test_paired_denominators_not_difference_of_means(self):
        summary=process(self.ctx())["data"]["comparison"]
        row=next(r for r in summary["rows"] if r["metric_id"]=="rag.f1" and r["candidate_id"]=="new")
        self.assertEqual(row["mean"],.2)
        self.assertEqual(row["paired"],1)
        self.assertEqual(row["delta"],.2)  # Not .2 - .5 = -.3.
        self.assertEqual(row["case_ids"],["q1"])
        cny=next(r for r in summary["rows"] if r["metric_id"]=="usage.cost" and r["candidate_id"]=="new" and r["parameters"]["currency"]=="CNY")
        self.assertIsNone(cny["delta"])

    def test_pending_and_no_fallback(self):
        ctx=self.ctx();ctx["cases"]=[]
        result=process(ctx)["data"]
        self.assertEqual(result["series"],[])
        self.assertEqual(result["comparison"]["candidates"][0]["processed"],0)
        self.assertEqual(result["comparison"]["rows"],[])

    def test_duplicate_metric_identity_rejected(self):
        ctx=self.ctx()
        ctx["cases"][0]["metrics"].append(dict(ctx["cases"][0]["metrics"][0]))
        with self.assertRaises(ValueError): process(ctx)
