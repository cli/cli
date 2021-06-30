from unittest import TestCase, main
from unittest.mock import patch

from pyrarcrack import generate_combinations


class TestCombination(TestCase):
    def test_should_generate_minimal_combination(self):
        self.assertEqual(
            list(generate_combinations('a', 1)),
            ['a']
        )


if __name__ == '__main__':
    main()
