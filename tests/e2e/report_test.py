"""보고서가 상위 실패를 중복 집계하거나 미실행을 통과로 표시하지 않는지 확인합니다."""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


class ReportTest(unittest.TestCase):
    def test_statuses_and_empty_run(self):
        report = Path(__file__).with_name('report.py')
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            log = root / 'test.jsonl'
            events = []
            for name, action in [('TestE2E', 'fail'), ('TestE2E/badger', 'fail'),
                                 ('TestE2E/badger/read', 'fail'),
                                 ('TestE2E/badger/write', 'pass'),
                                 ('TestE2E/badger/browser', 'skip')]:
                events.extend([{'Test': name, 'Action': 'run'},
                               {'Test': name, 'Action': action}])
            events.append({'Test': 'TestE2E/localstorage/interrupted', 'Action': 'run'})
            log.write_text('\n'.join(json.dumps(e) for e in events))
            subprocess.run([sys.executable, str(report), str(log), str(root)],
                           check=True, capture_output=True)
            rows = json.loads((root / 'results.json').read_text())
            self.assertCountEqual([r['status'] for r in rows],
                                  ['FAIL', 'PASS', 'BLOCKED', 'BLOCKED'])
            log.write_text('')
            subprocess.run([sys.executable, str(report), str(log), str(root)],
                           check=True, capture_output=True)
            self.assertEqual(json.loads((root / 'results.json').read_text())[0]['status'], 'BLOCKED')


if __name__ == '__main__':
    unittest.main()
