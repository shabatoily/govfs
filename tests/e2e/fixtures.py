"""기존 생성기 샘플을 보완하고 재현용 manifest를 저장합니다."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile


EXPECTED_MIME_TYPES = {
    'md': 'text/markdown',
    'txt': 'text/plain',
    'json': 'application/json',
    'xml': 'application/xml',
    'html': 'text/html',
    'htm': 'text/html',
    'js': 'text/javascript',
    'mjs': 'text/javascript',
    'css': 'text/css',
    'csv': 'text/csv',
    'jpg': 'image/jpeg',
    'jpeg': 'image/jpeg',
    'png': 'image/png',
    'webp': 'image/webp',
    'gif': 'image/gif',
    'svg': 'image/svg+xml',
    'mp4': 'video/mp4',
    'webm': 'video/webm',
    'avi': 'video/x-msvideo',
    'mov': 'video/quicktime',
    'mkv': 'video/x-matroska',
    'mpeg': 'video/mpeg',
    'mpg': 'video/mpeg',
    'mp3': 'audio/mpeg',
    'wav': 'audio/wav',
    'pdf': 'application/pdf',
}


def run_ffmpeg(args):
    subprocess.run(['ffmpeg', '-v', 'error', '-y', *args], check=True, stdout=subprocess.DEVNULL)


def validate_generator_outputs(fixture_dir):
    """기존 이미지·동영상·텍스트 생성기의 결과를 확인합니다."""
    for name in [
        'image_0_320x240.png',
        'image_1_320x240.png',
        'video_0_320x240_2s.mp4',
        'text_0_4096.txt',
        'text_1_4096.txt',
    ]:
        if not (fixture_dir / name).is_file():
            raise RuntimeError(f'Missing generator output: {name}')
    for name in ['text_0_4096.txt', 'text_1_4096.txt']:
        data = (fixture_dir / name).read_bytes()
        if len(data) != 4096 or any(byte < 32 or byte > 126 for byte in data):
            raise RuntimeError(f'Invalid text generator output: {name}')


def create_media(fixture_dir):
    """원본 PNG와 MP4를 실제 미디어 형식으로 변환합니다."""
    png = fixture_dir / 'image_0_320x240.png'
    mp4 = fixture_dir / 'video_0_320x240_2s.mp4'
    for extension in ['jpg', 'jpeg', 'gif']:
        run_ffmpeg(['-i', str(png), str(fixture_dir / ('sample.' + extension))])
    subprocess.run(['cwebp', '-quiet', str(png), '-o', str(fixture_dir / 'sample.webp')], check=True)
    (fixture_dir / 'UPPER.PNG').write_bytes(png.read_bytes())
    (fixture_dir / 'sample.svg').write_text(
        '<svg xmlns="http://www.w3.org/2000/svg" width="200" height="100">'
        '<rect width="200" height="100" fill="green"/>'
        '<text x="10" y="50">E2E SVG</text></svg>'
    )
    for extension, codec in [
        ('webm', 'libvpx'),
        ('avi', 'mpeg4'),
        ('mov', 'libx264'),
        ('mkv', 'libx264'),
        ('mpeg', 'mpeg2video'),
        ('mpg', 'mpeg2video'),
    ]:
        run_ffmpeg(['-i', str(mp4), '-c:v', codec, str(fixture_dir / ('sample.' + extension))])
    for extension in ['mp3', 'wav']:
        run_ffmpeg(['-f', 'lavfi', '-i', 'sine=frequency=440:duration=2', str(fixture_dir / ('sample.' + extension))])


def create_pdf(fixture_dir):
    """텍스트 한 줄을 담은 PDF의 객체와 참조 테이블을 작성합니다."""
    objects = [
        b'<< /Type /Catalog /Pages 2 0 R >>',
        b'<< /Type /Pages /Kids [3 0 R] /Count 1 >>',
        b'<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>',
        b'<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
    ]
    stream = b'BT /F1 24 Tf 30 100 Td (E2E PDF) Tj ET'
    objects.append(b'<< /Length ' + str(len(stream)).encode() + b' >>\nstream\n' + stream + b'\nendstream')
    pdf = b'%PDF-1.4\n'
    object_offsets = [0]
    for object_number, pdf_object in enumerate(objects, 1):
        object_offsets.append(len(pdf))
        pdf += str(object_number).encode() + b' 0 obj\n' + pdf_object + b'\nendobj\n'
    xref_offset = len(pdf)
    pdf += b'xref\n0 6\n0000000000 65535 f \n'
    for offset in object_offsets[1:]:
        pdf += f'{offset:010d} 00000 n \n'.encode()
    pdf += f'trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n{xref_offset}\n%%EOF\n'.encode()
    (fixture_dir / 'sample.pdf').write_bytes(pdf)


def create_text_and_archive(fixture_dir):
    """편집기 대상 확장자와 빈 파일·줄바꿈·크기 경계 샘플을 만듭니다."""
    for extension in [
        'txt',
        'md',
        'json',
        'xml',
        'html',
        'htm',
        'css',
        'js',
        'mjs',
        'csv',
        'ts',
        'go',
        'yaml',
        'unknown',
    ]:
        (fixture_dir / ('note.' + extension)).write_text('E2E 한글 😀\nline two\n')
    (fixture_dir / 'README').write_text('no extension\n')
    (fixture_dir / 'empty.txt').write_bytes(b'')
    (fixture_dir / 'crlf.txt').write_bytes(b'line1\r\nline2\r\n')
    (fixture_dir / 'long.txt').write_text('x' * 4096)
    (fixture_dir / 'large.txt').write_bytes(b'x' * (1 << 20))
    (fixture_dir / '한글 space.txt').write_text('한글 파일명\n')
    with zipfile.ZipFile(fixture_dir / 'sample.zip', 'w') as archive:
        archive.writestr('note.txt', 'e2e')


def write_manifest(output_dir, fixture_dir):
    """파일별 기대 MIME·렌더러·크기·해시를 기록합니다."""
    manifest = []
    for path in sorted(fixture_dir.iterdir()):
        data = path.read_bytes()
        mime = EXPECTED_MIME_TYPES.get(path.suffix[1:].lower(), 'application/octet-stream')
        media_type = mime.split('/')[0]
        if media_type in ('image', 'audio', 'video'):
            renderer = media_type
        elif mime == 'application/pdf':
            renderer = 'pdf'
        else:
            renderer = 'editor'
        manifest.append({
            'path': path.name,
            'size': len(data),
            'sha256': hashlib.sha256(data).hexdigest(),
            'mime': mime,
            'renderer': renderer,
        })
    (output_dir / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
    print('Fixtures:', len(manifest))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('output_dir', type=Path)
    args = parser.parse_args()
    output_dir = args.output_dir.resolve()
    fixture_dir = output_dir / 'fixtures'

    validate_generator_outputs(fixture_dir)
    create_media(fixture_dir)
    create_pdf(fixture_dir)
    create_text_and_archive(fixture_dir)
    write_manifest(output_dir, fixture_dir)


if __name__ == '__main__':
    main()
