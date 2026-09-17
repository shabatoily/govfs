"""go test -json 결과를 개별 사례 결과와 Markdown 보고서로 변환합니다."""
import argparse
import json
from collections import Counter
from pathlib import Path


def collect_results(log_path):
    events = [json.loads(line) for line in log_path.read_text().splitlines() if line.strip()]
    finished = {}
    outputs = {}
    started = set()
    for event in events:
        name = event.get('Test')
        if not name:
            continue
        if event['Action'] == 'run':
            started.add(name)
        if 'Output' in event:
            outputs.setdefault(name, []).append(event['Output'])
        if event['Action'] in ('pass', 'fail', 'skip'):
            finished[name] = event
    results = []
    for name in sorted(started):
        children = [child for child in started if child.startswith(name + '/')]
        event = finished.get(name, {})
        # 상위 그룹 실패는 하위 실패와 중복 집계하지 않습니다.
        if children:
            child_failed = any(
                finished.get(child, {}).get('Action') == 'fail' for child in children
            )
            if event.get('Action') != 'fail' or child_failed:
                continue
        status = {'pass': 'PASS', 'fail': 'FAIL', 'skip': 'BLOCKED'}.get(event.get('Action'), 'BLOCKED')
        parts = name.split('/')
        results.append({
            'case_id': '/'.join(parts[2:]) or name,
            'driver': parts[1] if len(parts) > 1 else 'setup',
            'status': status,
            'actual': ''.join(outputs.get(name, [])),
            'duration_ms': round(event.get('Elapsed', 0) * 1000),
            'evidence': [str(log_path.resolve())],
        })
    if not results:
        results.append({
            'case_id': 'setup',
            'driver': 'setup',
            'status': 'BLOCKED',
            'actual': 'No test cases ran; inspect build/package output in the JSONL log.',
            'duration_ms': 0,
            'evidence': [str(log_path.resolve())],
        })

    return results


def write_report(results, output_dir):
    output_dir.mkdir(parents=True, exist_ok=True)
    (output_dir / 'results.json').write_text(json.dumps(results, ensure_ascii=False, indent=2))
    counts = Counter(result['status'] for result in results)
    summary = ', '.join(
        f'{status}: {counts[status]}' for status in ('PASS', 'FAIL', 'BLOCKED')
    )
    lines = [
        '# E2E 실행 결과',
        '',
        summary,
        '',
        '브라우저 및 문서의 추가 수동 사례는 별도로 실행해야 합니다. '
        '이 보고서는 자동 실행 범위의 결과입니다.',
        '',
        '| 드라이버 | 사례 | 결과 |',
        '| --- | --- | --- |',
    ]
    for result in results:
        lines.append(f"| {result['driver']} | {result['case_id'].replace('|', '/')} | {result['status']} |")
    lines += ['', '## 실패·미실행 상세', '']
    for result in results:
        if result['status'] != 'PASS':
            lines += [
                f"### {result['driver']} / {result['case_id']}",
                '',
                '```text',
                result['actual'].replace('```', "'''"),
                '```',
                '',
            ]
    (output_dir / 'report.md').write_text('\n'.join(lines))
    print(json.dumps(dict(counts)))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('log', type=Path)
    parser.add_argument('out', type=Path)
    args = parser.parse_args()
    results = collect_results(args.log)
    write_report(results, args.out)


if __name__ == '__main__':
    main()
