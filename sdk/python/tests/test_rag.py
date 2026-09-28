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
        self.assertAlmostEqual(values["rag.f1",1],2/3)
        self.assertAlmostEqual(values["rag.f1",5],4/7)
        self.assertAlmostEqual(values["rag.ndcg",1],1/2)
        self.assertAlmostEqual(values["rag.map",1],1/2)
        self.assertEqual(values["rag.map",5],1)
        output=process({"cases":[{"case":{"id":"q"},"status":"completed","metrics":metrics}],"total":1,"input_digest":"x"})
        self.assertEqual(len([s for s in output["data"]["series"] if s["metric_id"]=="rag.precision"]),2)
        points=output["data"]["rag_quality"][0]["points"]
        self.assertEqual([p["k"] for p in points],[1,5])
        self.assertAlmostEqual(points[0]["f1"],2/3)

    def test_f1_missing_and_zero(self):
        for ctx, status, value in [
            (self.context([]), "not_computable", None),
            (self.context([],{"qrels":{"a":1}}), "observed", 0),
            (self.context([],{"qrels":{"a":0}}), "not_computable", None),
        ]:
            metrics=rag.evaluate(ctx)
            f1=[m for m in metrics if m["metric_id"]=="rag.f1"]
            self.assertEqual(len(f1),2)
            self.assertTrue(all(m["status"]==status and m["value"]==value for m in f1))
            result=process({"cases":[{"case":{"id":"q"},"status":"completed","metrics":metrics}],"total":1,"input_digest":"x"})
            self.assertEqual(result["data"]["rag_quality"][0]["points"][0]["f1"],value)

    def test_f1_macro_mean_not_harmonic_of_macro_means(self):
        cases=[]
        for cid, docs, refs in [("a",[{"id":"a","text":"a"}],{"qrels":{"a":1}}),
                               ("b",[{"id":"a","text":"a"}],{"qrels":{"a":1,"b":1,"c":1,"d":1}})]:
            ctx=self.context(docs,refs);ctx["case"]["id"]=cid;ctx["parameters"]={"k":[1]}
            cases.append({"case":{"id":cid},"status":"completed","metrics":rag.evaluate(ctx)})
        point=process({"cases":cases,"total":2,"input_digest":"x"})["data"]["rag_quality"][0]["points"][0]
        self.assertAlmostEqual(point["f1"],.7)
        self.assertNotAlmostEqual(point["f1"],2*point["precision"]*point["recall"]/(point["precision"]+point["recall"]))
        self.assertEqual(point["counts"]["f1"],{"observed":2,"total":2})

    def test_quality_identity_and_legacy_missing_f1(self):
        metrics=rag.evaluate(self.context([],{"qrels":{"a":1}}))
        metrics=[m for m in metrics if m["metric_id"]!="rag.f1"]
        second=[{**m,"evaluator":"another@2"} for m in metrics]
        result=process({"cases":[{"case":{"id":"q"},"status":"completed","metrics":metrics+second}],"total":1,"input_digest":"x"})
        self.assertEqual(len(result["data"]["rag_quality"]),2)
        self.assertTrue(all(p["f1"] is None for g in result["data"]["rag_quality"] for p in g["points"]))

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
