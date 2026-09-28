import unittest
from seven_evaluate import rag
from seven_evaluate.basic import process

class RAGTest(unittest.TestCase):
    def context(self, docs, refs=None):
        return {"case":{"id":"q", "references":refs or {}}, "result":{"retrieval":docs}, "status":"completed", "parameters":{"k":[1,5]}}

    def test_formulas_and_identity(self):
        metrics=rag.evaluate(self.context([{"id":"b","text":"b"},{"id":"a","text":"a"}],{"qrels":{"a":2,"b":1}}))
        values={(m["metric_id"],m["parameters"].get("k")):m["value"] for m in metrics}
        self.assertEqual(values["rag.precision",5],.4)
        self.assertEqual(values["rag.recall",1],.5)
        self.assertAlmostEqual(values["rag.ndcg",1],1/2)
        self.assertAlmostEqual(values["rag.map",1],1/2)
        self.assertEqual(values["rag.map",5],1)
        output=process({"cases":[{"case":{"id":"q"},"status":"completed","metrics":metrics}],"total":1,"input_digest":"x"})
        self.assertEqual(len([s for s in output["data"]["series"] if s["metric_id"]=="rag.precision"]),2)

    def test_missing_vs_zero(self):
        self.assertEqual(rag.evaluate(self.context([]))[0]["status"],"not_computable")
        self.assertEqual(rag.evaluate(self.context([],{"qrels":{"a":1}}))[0]["value"],0)
        ctx=self.context([],{"qrels":{"a":1}});del ctx["result"]["retrieval"]
        self.assertEqual(rag.evaluate(ctx)[0]["reason"],"evidence_missing")

    def test_duplicate_and_invalid_k(self):
        with self.assertRaises(ValueError): rag.evaluate(self.context([{"id":"a","text":"a"}]*2))
        ctx=self.context([]);ctx["parameters"]={"k":[0]}
        with self.assertRaises(ValueError): rag.evaluate(ctx)

    def test_citation_missing_locator_is_not_failed_rule(self):
        ctx=self.context([{"id":"a","text":"abc"}])
        ctx["result"]["citations"]=[{"document_id":"a"}]
        values={m["metric_id"]:m for m in rag.evaluate(ctx)}
        self.assertTrue(values["citation.resolvable"]["value"])
        self.assertEqual(values["citation.range"]["status"],"not_computable")
