#!/usr/bin/env python3
"""Create deterministic, offline documents for native profiling."""
import argparse
import pathlib
import struct
import zlib


def png(width, height, color):
    def chunk(kind, payload):
        return struct.pack('!I', len(payload)) + kind + payload + struct.pack('!I', zlib.crc32(kind + payload))
    row = b'\x00' + bytes(color) * width
    return (b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('!IIBBBBB', width, height, 8, 2, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(row * height)) + chunk(b'IEND', b''))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', nargs='?', default='build/perf/fixtures')
    args = parser.parse_args()
    output = pathlib.Path(args.directory)
    output.mkdir(parents=True, exist_ok=True)
    paragraph = 'A paragraph with **emphasis**, 中文正文 and a [link](https://example.com).\n\n'
    sources = {
        'normal.md': '# Reading\n\n' + paragraph * 20,
        'long.md': '# Reading\n\n' + paragraph * 5000,
        'tables.md': '# Reading\n\n' + '| Key | Value |\n|---|---|\n| one | two |\n\n' * 500,
        'aligned.md': '# Reading\n\n' + '<div align=center>\n\n## Heading\n\nAligned **paragraph**.\n\n</div>\n\n' * 500,
        'mermaid.md': '# Reading\n\n' + '```mermaid\nflowchart LR\n A --> B\n```\n\n' * 32,
    }
    images = '# Reading images\n\n'
    for i in range(6):
        name = f'image-{i}.png'
        (output / name).write_bytes(png(1600, 1200, (40 + i * 30, 90, 160)))
        images += f'Image {i+1}\n\n![Image {i+1}]({name})\n\n'
    sources['images.md'] = images
    for i in range(1, 5):
        sources[f'window-{i}.md'] = f'# Window {i}\n\n' + paragraph * 1000
    for name, source in sources.items():
        (output / name).write_text(source, encoding='utf-8')
    print(output.resolve())


if __name__ == '__main__':
    main()
