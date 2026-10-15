# Bundled typefaces

- `gmarketmedium.ttf`, `gmarketbold.ttf`: original Gmarket Sans files from
  [the official distribution](https://corp.gmarket.com/fonts/GmarketSansTTF.zip).
  Copyright and license are retained in the fonts and `gmarket-license.txt`.
- Their WOFF2 copies use the family name **Gosu UI**, respecting the reserved
  Gmarket Sans name. Outlines and coverage are unchanged.
- `chakrabold.ttf`: original Chakra Petch Bold from
  [Google Fonts](https://github.com/google/fonts/tree/main/ofl/chakrapetch).
  Its license is retained as `chakra-license.txt`.
- `counter.ttf`, `counter.woff2`: **Gosu Counter**, derived from Chakra Petch Bold.
  Decimal digits have equal advances and are centered in their cells, so animated
  scores never change column width. Other glyphs keep their original outlines.

`prepare.py` regenerates derived files using Python, fonttools and brotli.
Runtime builds use the committed files and need none of these tools or a network.
