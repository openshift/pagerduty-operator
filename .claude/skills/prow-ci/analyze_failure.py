#!/usr/bin/env python3
"""
Analyze Prow CI job failures from downloaded artifacts.
"""

import argparse
import json
import os
import re
import sys


def analyze_build_log(log_file):
    """Analyze build-log.txt for common failure patterns."""
    if not os.path.exists(log_file):
        return None

    analysis = {
        'errors': [],
        'warnings': [],
        'patterns': [],
        'summary': ''
    }

    with open(log_file, 'r', encoding='utf-8', errors='replace') as f:
        lines = f.readlines()

    # Common failure patterns
    patterns = [
        (r'FAIL\s+github\.com/', 'test_failure', 'Test package failure'),
        (r'--- FAIL:', 'test_failure', 'Individual test failure'),
        (r'Error: exit status \d+', 'exit_error', 'Command exit error'),
        (r'compilation failed', 'compile_error', 'Compilation error'),
        (r'cannot find package', 'import_error', 'Package import error'),
        (r'undefined:', 'undefined_error', 'Undefined reference'),
        (r'build failed', 'build_failure', 'Build failure'),
        (r'golangci-lint.*failed', 'lint_failure', 'Lint failure'),
        (r'timeout.*exceeded', 'timeout', 'Timeout exceeded'),
        (r'OOMKilled|out of memory', 'oom', 'Out of memory'),
        (r'panic:', 'panic', 'Go panic'),
    ]

    for pattern, key, description in patterns:
        matches = re.findall(pattern, content, re.IGNORECASE)
        if matches:
            analysis['patterns'].append({
                'type': key,
                'description': description,
                'count': len(matches)
            })

    # Extract error lines
    for i, line in enumerate(lines):
        if re.search(r'\bERROR\b|FAIL|panic:', line, re.IGNORECASE):
            context_start = max(0, i - 2)
            context_end = min(len(lines), i + 3)
            analysis['errors'].append({
                'line': i + 1,
                'content': line.strip(),
                'context': lines[context_start:context_end]
            })
            if len(analysis['errors']) >= 20:
                break

    # Generate summary
    if analysis['patterns']:
        pattern_types = [p['description'] for p in analysis['patterns']]
        analysis['summary'] = f"Detected: {', '.join(pattern_types)}"
    elif analysis['errors']:
        analysis['summary'] = f"Found {len(analysis['errors'])} error lines"
    else:
        analysis['summary'] = "No clear failure pattern detected"

    return analysis


def analyze_prowjob(prowjob_file):
    """Parse prowjob.json for job metadata."""
    if not os.path.exists(prowjob_file):
        return None

    try:
        with open(prowjob_file, 'r') as f:
            data = json.load(f)

        return {
            'name': data.get('spec', {}).get('job', 'unknown'),
            'state': data.get('status', {}).get('state', 'unknown'),
            'url': data.get('status', {}).get('url', ''),
            'type': data.get('spec', {}).get('type', 'unknown'),
        }
    except (json.JSONDecodeError, KeyError) as e:
        return {'error': str(e)}


def format_markdown(job_info, log_analysis):
    """Format analysis results as markdown."""
    lines = ['# CI Failure Analysis', '']

    if job_info:
        lines.append('## Job Information')
        lines.append(f"- **Name**: {job_info.get('name', 'N/A')}")
        lines.append(f"- **State**: {job_info.get('state', 'N/A')}")
        lines.append(f"- **Type**: {job_info.get('type', 'N/A')}")
        if job_info.get('url'):
            lines.append(f"- **URL**: {job_info['url']}")
        lines.append('')

    if log_analysis:
        lines.append('## Analysis')
        lines.append(f"**Summary**: {log_analysis['summary']}")
        lines.append('')

        if log_analysis['patterns']:
            lines.append('### Detected Patterns')
            for pattern in log_analysis['patterns']:
                lines.append(f"- **{pattern['description']}** ({pattern['count']} occurrences)")
            lines.append('')

        if log_analysis['errors']:
            lines.append(f"### Top Errors (showing {min(5, len(log_analysis['errors']))} of {len(log_analysis['errors'])})")
            for error in log_analysis['errors'][:5]:
                lines.append(f"\n**Line {error['line']}**: `{error['content']}`")
            lines.append('')

    return '\n'.join(lines)


def main():
    parser = argparse.ArgumentParser(description='Analyze Prow CI job failures')
    parser.add_argument('artifact_dir', help='Directory containing downloaded artifacts')
    parser.add_argument('-f', '--format', choices=['markdown', 'json', 'text'],
                        default='text', help='Output format (default: text)')

    args = parser.parse_args()

    if not os.path.isdir(args.artifact_dir):
        print(f"Error: {args.artifact_dir} is not a directory", file=sys.stderr)
        return 1

    prowjob_file = os.path.join(args.artifact_dir, 'prowjob.json')
    log_file = os.path.join(args.artifact_dir, 'build-log.txt')

    job_info = analyze_prowjob(prowjob_file)
    log_analysis = analyze_build_log(log_file)

    if job_info is None and log_analysis is None:
        print("Error: No artifacts could be parsed. Check that the artifact directory contains prowjob.json or build-log.txt.", file=sys.stderr)
        return 1

    if args.format == 'markdown':
        print(format_markdown(job_info, log_analysis))
    elif args.format == 'json':
        print(json.dumps({
            'job': job_info,
            'log_analysis': log_analysis
        }, indent=2))
    else:
        if job_info:
            print(f"Job: {job_info.get('name')} [{job_info.get('state')}]")
        if log_analysis:
            print(f"Summary: {log_analysis['summary']}")
            if log_analysis['patterns']:
                for p in log_analysis['patterns']:
                    print(f"  - {p['description']}: {p['count']} occurrences")

    return 0


if __name__ == '__main__':
    sys.exit(main())
