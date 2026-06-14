// Hand-written fixture inputs for scripts/generate-fixtures.ts. Each category lists named input HTML strings together with the schema JSON file
// (from model/testdata) they are parsed with. The generator runs every input through the reference prosemirror-model implementation and records
// the resulting document JSON and canonical HTML. The "test-dom" category ports parse test inputs from prosemirror/prosemirror-model/test/test-dom.ts
// (case names match the upstream test descriptions); all other categories use the example editor schema (example-schema.json).

export interface FixtureCase {
  name: string
  input: string
}

export interface CaseCategory {
  fixture: string
  schema: string
  cases: FixtureCase[]
}

export const caseCategories: CaseCategory[] = [
  {
    fixture: "nodes",
    schema: "example-schema.json",
    cases: [
      { name: "paragraph", input: "<p>hello</p>" },
      { name: "blockquote without cite", input: "<blockquote><p>quoted</p></blockquote>" },
      { name: "blockquote with cite", input: '<blockquote cite="https://example.com/source"><p>quoted</p></blockquote>' },
      { name: "heading level 1", input: "<h1>one</h1>" },
      { name: "heading level 2", input: "<h2>two</h2>" },
      { name: "heading level 3", input: "<h3>three</h3>" },
      { name: "heading level 4", input: "<h4>four</h4>" },
      { name: "heading drops marks", input: "<h2>plain <b>bold</b> <i>italic</i></h2>" },
      { name: "preformatted", input: "<pre>code here</pre>" },
      { name: "bullet list", input: "<ul><li>one</li><li>two</li></ul>" },
      { name: "ordered list", input: "<ol><li>first</li><li>second</li></ol>" },
      { name: "nested bullet list", input: "<ul><li>one<ul><li>nested</li></ul></li><li>two</li></ul>" },
      { name: "ordered list nested in bullet list", input: "<ul><li>one<ol><li>sub</li></ol></li></ul>" },
      { name: "list item with multiple paragraphs", input: "<ul><li><p>first</p><p>second</p></li></ul>" },
      { name: "horizontal rule at top level", input: "<p>before</p><hr><p>after</p>" },
      { name: "horizontal rule nested in a list is dropped", input: "<ul><li>item<hr></li></ul>" },
      { name: "horizontal rule nested in a blockquote is dropped", input: "<blockquote><p>a</p><hr><p>b</p></blockquote>" },
      { name: "hard break", input: "<p>one<br>two</p>" },
      { name: "empty input", input: "" },
      { name: "plain text without tags", input: "plain text" },
      { name: "whitespace-only input", input: "   \n\t  " },
    ],
  },
  {
    fixture: "marks",
    schema: "example-schema.json",
    cases: [
      { name: "bold", input: "<p><b>bold</b></p>" },
      { name: "italic", input: "<p><i>italic</i></p>" },
      { name: "underline", input: "<p><u>underlined</u></p>" },
      { name: "strikethrough", input: "<p><strike>struck</strike></p>" },
      { name: "monospace", input: "<p><tt>mono</tt></p>" },
      { name: "link", input: '<p><a href="https://example.com/">link</a></p>' },
      { name: "bold italic nesting", input: "<p><b><i>both</i></b></p>" },
      { name: "italic bold nesting", input: "<p><i><b>both</b></i></p>" },
      { name: "adjacent same-mark text merges", input: "<p><b>one</b><b>two</b></p>" },
      { name: "marks across hard break", input: "<p><b>one<br>two</b></p>" },
      { name: "overlapping mark ranges", input: "<p><b>a<i>b</i></b><i>c</i></p>" },
      { name: "strong parses as bold", input: "<p><strong>x</strong></p>" },
      { name: "em parses as italic", input: "<p><em>x</em></p>" },
      { name: "s parses as strikethrough", input: "<p><s>x</s></p>" },
      { name: "del parses as strikethrough", input: "<p><del>x</del></p>" },
    ],
  },
  {
    fixture: "whitespace",
    schema: "example-schema.json",
    cases: [
      { name: "runs of whitespace collapse to one space", input: "<p>a  \t b\n\nc</p>" },
      { name: "leading whitespace at block start dropped", input: "<p>   hello</p>" },
      { name: "whitespace after hard break dropped", input: "<p>one<br>   two</p>" },
      { name: "trailing whitespace at block end stripped", input: "<p>hello   </p>" },
      // The characters between "a" and "b" are two literal U+00A0 (no-break space) characters.
      { name: "non-breaking space preserved verbatim", input: "<p>a  b</p>" },
      { name: "whitespace between block elements ignored", input: "<p>one</p>  \n  <p>two</p>" },
      { name: "pre keeps all whitespace", input: "<pre>  one\t two\nthree  </pre>" },
      { name: "pre normalizes crlf to lf", input: "<pre>one\r\ntwo\rthree</pre>" },
    ],
  },
  {
    fixture: "escaping",
    schema: "example-schema.json",
    cases: [
      { name: "escaped characters in text", input: "<p>&amp; &lt; &gt; \" ' raw</p>" },
      { name: "escaped characters in attribute value", input: "<p><a href=\"/q?a=&amp;b=<c>&amp;d='e'&amp;f=&quot;g&quot;\">x</a></p>" },
      // The character between "nbsp" and "here" is a literal U+00A0 (no-break space) character.
      { name: "non-breaking space stays raw", input: "<p>nbsp here</p>" },
      { name: "emoji stays raw", input: "<p>emoji \u{1F600} stays</p>" },
      { name: "pre-escaped angle brackets stay text", input: "<p>&lt;div&gt; is text</p>" },
    ],
  },
  {
    fixture: "pre",
    schema: "example-schema.json",
    cases: [
      { name: "leading newline dropped by the HTML parser", input: "<pre>\nx</pre>" },
      { name: "double leading newline keeps one", input: "<pre>\n\nx</pre>" },
      { name: "trailing newline preserved", input: "<pre>x\n</pre>" },
      { name: "marks stripped inside pre", input: "<pre>plain <b>bold</b> <i>italic</i></pre>" },
    ],
  },
  {
    fixture: "blockquote",
    schema: "example-schema.json",
    cases: [
      { name: "paragraph becomes blockquote paragraph", input: "<blockquote><p>text</p></blockquote>" },
      { name: "italic dropped inside blockquote", input: "<blockquote><p>a<i>b</i>c</p></blockquote>" },
      { name: "bold kept inside blockquote", input: "<blockquote><p><b>bold</b></p></blockquote>" },
      { name: "valid cite kept", input: '<blockquote cite="/sources/1"><p>x</p></blockquote>' },
      { name: "invalid cite dropped blockquote kept", input: '<blockquote cite="javascript:alert(1)"><p>x</p></blockquote>' },
      { name: "mailto cite rejected", input: '<blockquote cite="mailto:user@example.com"><p>x</p></blockquote>' },
      { name: "valid mailto link inside blockquote", input: '<blockquote><p><a href="mailto:user@example.com">mail</a></p></blockquote>' },
    ],
  },
  {
    fixture: "links",
    schema: "example-schema.json",
    cases: [
      { name: "invalid javascript href", input: '<p><a href="javascript:alert(1)">x</a></p>' },
      { name: "invalid file href", input: '<p><a href="file:///etc/passwd">x</a></p>' },
      { name: "invalid data href", input: '<p><a href="data:text/html;base64x">x</a></p>' },
      { name: "invalid relative href", input: '<p><a href="foo">x</a></p>' },
      { name: "invalid protocol-relative href", input: '<p><a href="//host/x">x</a></p>' },
      { name: "invalid http href with empty host", input: '<p><a href="http:///x">x</a></p>' },
      { name: "invalid bare mailto href", input: '<p><a href="mailto:">x</a></p>' },
      { name: "invalid empty href", input: '<p><a href="">x</a></p>' },
      { name: "anchor without href", input: "<p><a>x</a></p>" },
      { name: "valid absolute path href", input: '<p><a href="/path">x</a></p>' },
      { name: "valid root path href", input: '<p><a href="/">x</a></p>' },
      { name: "valid http href", input: '<p><a href="http://example.com/">x</a></p>' },
      { name: "valid https href with query", input: '<p><a href="https://example.com/x?y=1&amp;z=2">x</a></p>' },
      { name: "valid uppercase mailto href", input: '<p><a href="MAILTO:a@b.c">x</a></p>' },
      { name: "valid uppercase https href", input: '<p><a href="HTTPS://EXAMPLE.COM/">x</a></p>' },
    ],
  },
  {
    fixture: "word-paste",
    schema: "example-schema.json",
    cases: [
      { name: "mso paragraph styles ignored", input: '<p class="MsoNormal" style="mso-margin-top-alt:auto;mso-margin-bottom-alt:auto">Word text</p>' },
      { name: "o:p tags skipped", input: "<p>Hello<o:p></o:p> world</p>" },
      { name: "conditional comments ignored", input: "<p><!--[if !supportLists]-->* <!--[endif]-->Item</p>" },
      { name: "font tags skipped content kept", input: '<p><font face="Calibri" size="2">styled</font> text</p>' },
      { name: "span with bold style skipped plain text kept", input: '<p><span style="font-weight:bold">important</span> rest</p>' },
      { name: "div wrappers unwrapped", input: "<div><div><p>nested</p></div><div>loose</div></div>" },
      {
        name: "word list paragraph paste",
        input:
          '<p class="MsoListParagraph" style="text-indent:-18.0pt;mso-list:l0 level1 lfo1">' +
          '<!--[if !supportLists]--><span style="mso-list:Ignore">1.<span style="font:7.0pt &quot;Times New Roman&quot;">&nbsp; </span></span><!--[endif]-->' +
          "First item<o:p></o:p></p>",
      },
    ],
  },
  {
    fixture: "structure",
    schema: "example-schema.json",
    cases: [
      { name: "stray inline content wrapped in paragraph", input: "text <b>bold</b> tail" },
      { name: "list item outside a list wrapped", input: "<li>stray</li>" },
      { name: "table content parsed without table structure", input: "<table><tr><td>cell one</td><td>cell two</td></tr></table>" },
      { name: "headings h5 and h6 fall back to paragraph", input: "<h5>five</h5><h6>six</h6>" },
      { name: "ul directly inside ul", input: "<ul><ul><li>deep</li></ul></ul>" },
      { name: "list normalization folds sibling list into previous item", input: "<ul><li>one</li><ul><li>nested</li></ul></ul>" },
      { name: "deeply nested unknown inline elements", input: "<p><span><span>x</span></span></p>" },
    ],
  },
  {
    fixture: "test-dom",
    schema: "basic-schema.json",
    cases: [
      { name: "can represent simple node", input: "<p>hello</p>" },
      { name: "can represent a line break", input: "<p>hi<br/>there</p>" },
      { name: "can represent an image", input: '<p>hi<img src="img.png" alt="x"/>there</p>' },
      { name: "joins styles", input: "<p>one<strong>two</strong><em><strong>three</strong>four</em>five</p>" },
      { name: "can represent links", input: '<p>a <a href="foo">big </a><a href="bar">nested</a><a href="foo"> link</a></p>' },
      { name: "can represent and unordered list", input: "<ul><li><p>one</p></li><li><p>two</p></li><li><p>three<strong>!</strong></p></li></ul><p>after</p>" },
      { name: "can represent an ordered list", input: "<ol><li><p>one</p></li><li><p>two</p></li><li><p>three<strong>!</strong></p></li></ol><p>after</p>" },
      { name: "can represent a blockquote", input: "<blockquote><p>hello</p><p>bye</p></blockquote>" },
      { name: "can represent a nested blockquote", input: "<blockquote><blockquote><blockquote><p>he said</p></blockquote></blockquote><p>i said</p></blockquote>" },
      { name: "can represent headings", input: "<h1>one</h1><h2>two</h2><p>text</p>" },
      { name: "can represent a code block", input: "<blockquote><pre><code>some code</code></pre></blockquote><p>and</p>" },
      { name: "can represent inline code", input: "<p>text and <code>code that is </code><em><code>emphasized</code></em><code>...</code></p>" },
      { name: "supports leaf nodes in marks", input: "<p><em>hi<br>x</em></p>" },
      { name: "doesn't collapse non-breaking spaces", input: "<p>   hello </p>" },
      { name: "can recover a list item", input: "<ol><p>Oh no</p></ol>" },
      { name: "wraps a list item in a list", input: "<li>hey</li>" },
      { name: "can turn divs into paragraphs", input: "<div>hi</div><div>bye</div>" },
      { name: "interprets <i> and <b> as emphasis and strong", input: "<p><i>hello <b>there</b></i></p>" },
      { name: "wraps stray text in a paragraph", input: "hi" },
      { name: "ignores an extra wrapping <div>", input: "<div><p>one</p><p>two</p></div>" },
      { name: "ignores meaningless whitespace", input: " <blockquote> <p>woo  \n  <em> hooo</em></p> </blockquote> " },
      { name: "removes whitespace after a hard break", input: "<p>hello<br>\n  world</p>" },
      { name: "converts br nodes to newlines when they would otherwise be ignored", input: "<pre>foo<br>bar</pre>" },
      { name: "finds a valid place for invalid content", input: "<ul><li>hi</li><p>whoah</p><li>again</li></ul>" },
      { name: "moves nodes up when they don't fit the current context", input: "<div>hello<hr/>bye</div>" },
      { name: "doesn't ignore whitespace-only text nodes", input: "<p><em>one</em> <strong>two</strong></p>" },
      { name: "can handle stray tab characters", input: "<p> <b>&#09;</b></p>" },
      { name: "normalizes random spaces", input: "<p><b>1 </b>  </p>" },
      { name: "can parse an empty code block", input: "<pre></pre>" },
      { name: "preserves trailing space in a code block", input: "<pre>foo\n</pre>" },
      { name: "ignores <script> tags", input: "<p>hello<script>alert('x')</script>!</p>" },
      { name: "can handle a head/body input structure", input: "<head><title>T</title><meta charset='utf8'/></head><body>hi</body>" },
      { name: "only applies a mark once", input: "<p>A <strong>big <strong>strong</strong> monster</strong>.</p>" },
      { name: "ignores unknown inline tags", input: "<p><u>a</u>bc</p>" },
      { name: "can add marks specified before their parent node is opened", input: "<em>hi</em> you" },
      { name: "keeps applying a mark for the all of the node's content", input: "<p><strong><span>xx</span>bar</strong></p>" },
      { name: "closes block with inline content on seeing block-level children", input: "<div><br><div>CCC</div><div>DDD</div><br></div>" },
      { name: "doesn't get confused by nested mark tags", input: "<div><strong><strong>A</strong></strong>B</div><span>C</span>" },
      { name: "preserves whitespace in nodes styled with white-space", input: "  <div style='white-space: pre'>  okay  then </div>  <p> x</p>" },
    ],
  },
  {
    fixture: "whitespace-style",
    schema: "basic-schema.json",
    cases: [
      // The white-space style value is matched against the CSSOM-normalized keyword: the property name is case-sensitive lowercase, the value is
      // lowercased, and invalid keyword values do not preserve whitespace. These cases pin that the Go detection agrees with the reference CSSOM.
      { name: "white-space pre preserves", input: "<p style='white-space: pre'>a  b</p>" },
      { name: "white-space uppercase value preserves", input: "<p style='white-space: PRE'>a  b</p>" },
      { name: "white-space uppercase property collapses", input: "<p style='WHITE-SPACE: pre'>a  b</p>" },
      { name: "white-space invalid value collapses", input: "<p style='white-space:prewrap'>a  b</p>" },
      { name: "white-space mixed-case pre-wrap preserves", input: "<p style='white-space: Pre-Wrap'>a  b</p>" },
      { name: "white-space pre-line preserves", input: "<p style='white-space: pre-line'>a  b</p>" },
      { name: "white-space normal collapses", input: "<p style='white-space: normal'>a  b</p>" },
      { name: "white-space nowrap collapses", input: "<p style='white-space: nowrap'>a  b</p>" },
    ],
  },
  {
    fixture: "custom-marks",
    schema: "custom-marks-schema.json",
    cases: [
      // A non-spanning mark (spanning false) closes and reopens around every node, so the single parsed test mark over the text and image serializes as
      // three separate test elements, mirroring the upstream "serializes non-spanning marks correctly" case.
      { name: "serializes non-spanning marks correctly", input: '<p><test>a<img src="x">b</test></p>' },
    ],
  },
  {
    fixture: "feature",
    schema: "feature-schema.json",
    cases: [
      // Class selector: "p.foo" matches only a paragraph with class foo (parsed as a callout), while "p" matches any other paragraph regardless of class.
      { name: "class selector matches only the class", input: '<p class="foo">a</p><p class="bar">b</p><p>c</p>' },
      // Style rule: an inline longhand font-weight: bold declaration applies the bold mark. The shorthand counterpart (style="font: bold ...") is the one
      // documented fidelity boundary of the port and is asserted separately in a Go test, since the reference matches it through CSSOM shorthand expansion.
      { name: "style rule matches inline longhand", input: '<p><span style="font-weight: bold">x</span></p>' },
      // Namespace rule: a foreign svg element matches the namespaced rule; an svg child element with no matching node type is dropped while text is kept.
      { name: "namespace rule matches foreign element", input: "<svg><circle></circle>shape</svg>" },
      // A foreign-namespaced script (and style) is fully ignored, so its body never leaks as text.
      { name: "foreign namespaced script is ignored", input: "<svg><script>alert(1)</script><style>x{}</style>ok</svg>" },
    ],
  },
  {
    // The serializable parse rule flags (consuming, ignore, skip, closeParent) exercised through the schema dialect. The flags-schema attaches each flag to a
    // parse rule and these cases observe its effect on the parsed document.
    fixture: "flags",
    schema: "flags-schema.json",
    cases: [
      // closeParent: a br closes the enclosing paragraph, so the text after it begins a new paragraph.
      { name: "close parent splits the paragraph", input: "<p>one<br>two</p>" },
      // skip: the cite element itself is dropped but its content is parsed in place.
      { name: "skip drops the element but keeps its content", input: "<p>a<cite>b</cite>c</p>" },
      // ignore: the del element is dropped together with its content.
      { name: "ignore drops the element and its content", input: "<p>a<del>b</del>c</p>" },
      // consuming false on a style rule: the font-weight rule applies em without ending the search, so the font-weight=800 rule then applies strong.
      { name: "non-consuming style rule applies a second mark", input: '<p><span style="font-weight: 800">one</span></p>' },
      // ignore on a style rule: an element carrying the matched inline style is dropped with its content.
      { name: "ignore style rule drops the element", input: '<p>x<span style="font-style: oblique">y</span>z</p>' },
    ],
  },
  {
    fixture: "linebreak",
    schema: "linebreak-schema.json",
    cases: [
      // The schema has no pre node, so a pre element is unknown, but the pre tag name still preserves the whitespace of its text, which is then wrapped in a
      // paragraph. Mirrors the upstream "preserves whitespace in <pre> elements" case (the schema also carries a linebreakReplacement hard_break, which is
      // irrelevant here since there are no newlines).
      { name: "preserves whitespace in pre elements", input: "<pre>  hello </pre>   " },
      // With a linebreakReplacement hard_break, newlines in preserved-whitespace text become hard breaks. Mirrors the first assertion of the upstream
      // "inserts line break replacements" case.
      { name: "inserts line break replacements", input: "<p><span style='white-space: pre'>one\ntwo\n\nthree</span></p>" },
      // Without white-space pre the same newlines collapse to spaces instead, the second assertion of the upstream case.
      { name: "collapses newlines without preserved whitespace", input: "<p><span>one\ntwo\n\nthree</span></p>" },
    ],
  },
  {
    // Representative real-world document content, pinning that typical rich text is canonical or canonicalizes as expected: plain text rendered to HTML,
    // linkified URLs and emails, file attachment links, bare inline content wrapped into a paragraph, fully disallowed input reduced to empty, and
    // descriptive prose with links.
    fixture: "documents",
    schema: "example-schema.json",
    cases: [
      { name: "apostrophe entity", input: "<p>L&#39;art pour l&#39;art</p>" },
      { name: "newline as hard break", input: "<p>line1<br>line2</p>" },
      { name: "linkified url", input: '<p>vir: <a href="http://www.mg-lj.si">www.mg-lj.si</a></p>' },
      { name: "linkified email", input: '<p>kontakt: <a href="mailto:info@mg-lj.si">info@mg-lj.si</a></p>' },
      { name: "attachment link paragraph", input: '<p>opomba</p><p><a href="/f/6XmpqAEPHRGoWB3cBdrvih">scan.pdf</a></p>' },
      { name: "multiple attachment links", input: '<p><a href="/f/a">one.pdf</a><br><a href="/f/b">two.pdf</a></p>' },
      { name: "bare inline wraps into a paragraph", input: "<b>bold</b>" },
      { name: "all-disallowed input becomes empty", input: "<script>alert(1)</script>" },
      { name: "script with surrounding text", input: "hello <script>alert(1)</script> world" },
      { name: "description block", input: "<p>A document describes a property.</p>" },
      { name: "description with link", input: '<p>Data from <a href="https://www.geonames.org/">GeoNames</a>.</p>' },
    ],
  },
]
