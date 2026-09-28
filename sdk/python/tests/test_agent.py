import unittest
from seven_evaluate import agent

class AgentTest(unittest.TestCase):
    def test_call_is_not_task_effect(self):
        ctx={"case":{"id":"q","references":{"expected_tools":[{"name":"search","arguments":{"q":"x"}}]}},"status":"completed","result":{"response":"ok","tools":[{"id":"t1","name":"search","arguments":{"q":"x"},"status":"completed"}]}}
        metrics={m["metric_id"]:m for m in agent.evaluate(ctx)}
        self.assertTrue(metrics["agent.call.completed"]["value"])
        self.assertTrue(metrics["tool.expected"]["value"])
        self.assertEqual(metrics["agent.effect"]["status"],"not_computable")
        ctx["result"]["tools"].append({"id":"t2","name":"search","arguments":{"q":"x"},"status":"error"})
        metrics={m["metric_id"]:m for m in agent.evaluate(ctx)}
        self.assertFalse(metrics["tool.execution"]["value"])
        self.assertEqual(metrics["tool.repeated"]["value"],1)

    def test_missing_tool_arguments_are_not_failed_matching(self):
        ctx={"case":{"id":"q","references":{"expected_tools":[{"name":"search","arguments":{"q":"x"}}]}},"status":"completed","result":{"tools":[{"id":"t","name":"search","status":"completed"}]}}
        metrics={m["metric_id"]:m for m in agent.evaluate(ctx)}
        self.assertTrue(metrics["tool.expected"]["value"])
        self.assertEqual(metrics["tool.arguments"]["status"],"not_computable")
        self.assertIsNone(metrics["tool.arguments"]["value"])
