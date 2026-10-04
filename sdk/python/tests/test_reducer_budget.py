import unittest

from seven_evaluate.reducer import OutputBudget


class ReducerBudgetTest(unittest.TestCase):
    def test_report_budget_fails_instead_of_truncating(self):
        budget = OutputBudget()
        budget.remaining = 8
        self.assertEqual(budget.add(0), 0)
        self.assertIs(budget.add(False), False)
        with self.assertRaisesRegex(ValueError, "budget exceeded"):
            budget.add("oversize")

    def test_nonfinite_statistic_is_rejected(self):
        with self.assertRaises(ValueError):
            OutputBudget().add(float("inf"))
