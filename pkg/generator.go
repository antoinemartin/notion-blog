package notion_blog

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"net/url"

	"unicode"
	"unicode/utf8"

	"github.com/gosimple/slug"
	"github.com/janeczku/go-spinner"
	"github.com/jomei/notionapi"
)

func wordWrap(text string, lineWidth int, prefix string) string {
	wrap := make([]byte, 0, len(text)+2*len(text)/lineWidth)
	eoLine := lineWidth - len(prefix)
	inWord := false
	for i, j := 0, 0; ; {
		r, size := utf8.DecodeRuneInString(text[i:])
		if size == 0 && r == utf8.RuneError {
			r = ' '
		}
		if unicode.IsSpace(r) {
			if inWord {
				if i > eoLine {
					wrap = append(wrap, '\n')
					eoLine = len(wrap) + lineWidth - len(prefix)
					wrap = append(wrap, []byte(prefix)...)
				} else if len(wrap) > 0 {
					wrap = append(wrap, ' ')
				}
				wrap = append(wrap, text[j:i]...)
			}
			inWord = false
		} else if !inWord {
			inWord = true
			j = i
		}
		if size == 0 && r == ' ' {
			break
		}
		i += size
	}
	return string(wrap)
}

func emphFormat(a *notionapi.Annotations) (s string) {
	s = "%s"
	if a == nil {
		return
	}

	if a.Code {
		return "`%s`"
	}

	switch {
	case a.Bold && a.Italic:
		s = "***%s***"
	case a.Bold:
		s = "**%s**"
	case a.Italic:
		s = "*%s*"
	}

	if a.Underline {
		s = "__" + s + "__"
	} else if a.Strikethrough {
		s = "~~" + s + "~~"
	}

	// TODO: color

	return s
}

func ConvertRich(t notionapi.RichText) string {
	switch t.Type {
	case notionapi.ObjectTypeText:
		if t.Text.Link != nil {
			return fmt.Sprintf(
				emphFormat(t.Annotations),
				fmt.Sprintf("[%s](%s)", t.Text.Content, t.Text.Link.Url),
			)
		}
		return fmt.Sprintf(emphFormat(t.Annotations), t.Text.Content)
	case notionapi.ObjectTypeList:
	}
	return ""
}

func ConvertRichText(t []notionapi.RichText) string {
	buf := &bytes.Buffer{}
	for _, word := range t {
		buf.WriteString(ConvertRich(word))
	}

	return strings.TrimSpace(buf.String())
}

func getImage(imgURL string, config BlogConfig) (_ string, err error) {
	// Split image url to get host and file name
	splittedURL, err := url.Parse(imgURL)
	if err != nil {
		return "", fmt.Errorf("malformed url: %s", err)
	}

	// Get file name
	filePath := splittedURL.Path
	filePath = filePath[strings.LastIndex(filePath, "/")+1:]

	name := fmt.Sprintf("%s_%s", splittedURL.Hostname(), filePath)

	spin := spinner.StartNew(fmt.Sprintf("Getting image `%s`", name))
	defer func() {
		spin.Stop()
		if err != nil {
			fmt.Printf("❌ Getting image `%s`: %s\n", name, err)
		} else {
			fmt.Printf("✔ Getting image `%s`: Completed\n", name)
		}
	}()

	resp, err := http.Get(imgURL)
	if err != nil {
		return "", fmt.Errorf("couldn't download image: %s", err)
	}
	defer resp.Body.Close()

	err = os.MkdirAll(config.ImagesFolder, 0777)
	if err != nil {
		return "", fmt.Errorf("couldn't create images folder: %s", err)
	}

	// Create the file
	out, err := os.Create(filepath.Join(config.ImagesFolder, name))
	if err != nil {
		return name, fmt.Errorf("couldn't create image file: %s", err)
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return filepath.Join(config.ImagesLink, name), err
}

func Generate(w io.Writer, page notionapi.Page, blocks []notionapi.Block, config BlogConfig) error {
	// Parse template file
	t := template.New(path.Base(config.ArchetypeFile)).Delims("[[", "]]")
	t.Funcs(template.FuncMap{
		"add":    func(a, b int) int { return a + b },
		"sub":    func(a, b int) int { return a - b },
		"mul":    func(a, b int) int { return a * b },
		"div":    func(a, b int) int { return a / b },
		"repeat": func(s string, n int) string { return strings.Repeat(s, n) },
		"rich":   ConvertRichText,
		"slug":   func(s string) string { return slug.Make(s) },
	})

	t, err := t.ParseFiles(config.ArchetypeFile)
	if err != nil {
		return fmt.Errorf("error parsing archetype file: %s", err)
	}

	// Generate markdown content
	buffer := &bytes.Buffer{}
	GenerateContent(buffer, blocks, config)

	// Dump markdown content into output according to archetype file
	fileArchetype := MakeArchetypeFields(page, config)
	fileArchetype.Content = buffer.String()
	err = t.Execute(w, fileArchetype)
	if err != nil {
		return fmt.Errorf("error filling archetype file: %s", err)
	}

	return nil
}

func EmojiToName(emoji notionapi.Emoji) string {
	switch emoji {
	case "⚠️":
		return "warning"
	case "💡":
		return "tip"
	case "🐞":
		return "bug"
	case "❓", "❔":
		return "question"
	case "❌", "🧨", "💣":
		return "failure"
	case "✅", "🆗", "☑️", "✔️":
		return "success"
	case "☠️", "⛔", "🛑":
		return "danger"
	case "📋":
		return "abstract"
	case "💬":
		return "quote"
	case "ℹ️":
		return "info"
	case "✍️":
		return "example"
	default:
		return "note"
	}
}

func GenerateContent(w io.Writer, blocks []notionapi.Block, config BlogConfig, prefixes ...string) {
	if len(blocks) == 0 {
		return
	}

	numberedList := false
	bulletedList := false

	lastIndex := len(blocks) - 1

	for blkIdx, block := range blocks {
		// Add line break after list is finished
		if bulletedList && block.GetType() != notionapi.BlockTypeBulletedListItem {
			bulletedList = false
			fmt.Fprintln(w)
		}
		if numberedList && block.GetType() != notionapi.BlockTypeNumberedListItem {
			numberedList = false
			fmt.Fprintln(w)
		}

		switch b := block.(type) {
		case *notionapi.ParagraphBlock:
			para := ConvertRichText(b.Paragraph.RichText)
			if para != "" && para != "\n" {
				fprintln(w, prefixes, wordWrap(para, 80, ""))
				if blkIdx < lastIndex {
					fprintf(w, prefixes, "")
				}
			}

			GenerateContent(w, b.Paragraph.Children, config)
		case *notionapi.Heading1Block:
			fprintf(w, prefixes, "# %s\n", ConvertRichText(b.Heading1.RichText))
		case *notionapi.Heading2Block:
			fprintf(w, prefixes, "## %s\n", ConvertRichText(b.Heading2.RichText))
		case *notionapi.Heading3Block:
			fprintf(w, prefixes, "### %s\n", ConvertRichText(b.Heading3.RichText))
		case *notionapi.CalloutBlock:
			// TODO: Instead of admonition, this should be {{<callout>}}
			// And have the shortcode interpreted inside the hugo theme.
			if !config.UseShortcodes {
				continue
			}
			if b.Callout.Icon != nil {
				if b.Callout.Icon.Emoji != nil {
					fprintf(w, prefixes, "{{< admonition %s >}}\n", EmojiToName(*b.Callout.Icon.Emoji))
				} else {
					fprintf(w, prefixes, "{{< admonition note >}}\n")
				}
			}
			fprintln(w, prefixes, ConvertRichText(b.Callout.RichText))
			GenerateContent(w, b.Callout.Children, config, prefixes...)
			fprintln(w, prefixes, "{{< /admonition >}}\n")

		case *notionapi.BookmarkBlock:
			if !config.UseShortcodes {
				// Simply generate the url link
				fprintf(w, prefixes, "[%s](%s)", b.Bookmark.URL, b.Bookmark.URL)
				continue
			}
			// Parse external page metadata
			og, err := parseMetadata(b.Bookmark.URL, config)
			if err != nil {
				log.Println("error getting bookmark metadata:", err)
			}

			// GenerateContent shortcode with given metadata
			fprintf(w, prefixes,
				`{{< bookmark url="%s" title="%s" img="%s" >}}%s{{< /bookmark >}}`,
				og.URL,
				og.Title,
				og.Image,
				og.Description,
			)

		case *notionapi.QuoteBlock:
			fprintf(w, prefixes, "> %s", wordWrap(ConvertRichText(b.Quote.RichText), 80, strings.Join(append(prefixes, "> "), "")))
			GenerateContent(w, b.Quote.Children, config,
				append([]string{"> "}, prefixes...)...)
			fprintln(w, prefixes)

		case *notionapi.BulletedListItemBlock:
			bulletedList = true
			fprintf(w, prefixes, "- %s", wordWrap(ConvertRichText(b.BulletedListItem.RichText), 80, strings.Join(append(prefixes, "  "), "")))
			GenerateContent(w, b.BulletedListItem.Children, config,
				append([]string{"    "}, prefixes...)...)

		case *notionapi.NumberedListItemBlock:
			numberedList = true
			fprintf(w, prefixes, "1. %s", ConvertRichText(b.NumberedListItem.RichText))
			GenerateContent(w, b.NumberedListItem.Children, config,
				append([]string{"    "}, prefixes...)...)

		case *notionapi.ImageBlock:
			src, _ := getImage(b.Image.File.URL, config)
			caption := ConvertRichText(b.Image.Caption)
			if caption == "" {
				caption = "image"
			}
			fprintf(w, prefixes, "![%s](%s)\n", caption, src)

		case *notionapi.CodeBlock:
			if b.Code.Language == "plain text" {
				fprintln(w, prefixes, "```")
			} else {
				fprintf(w, prefixes, "```%s", b.Code.Language)
			}
			fprintln(w, prefixes, ConvertRichText(b.Code.RichText))
			fprintln(w, prefixes, "```\n")

		case *notionapi.UnsupportedBlock:
			if b.GetType() != "unsupported" {
				fmt.Println("ℹ Unimplemented block", b.GetType())
			} else {
				fmt.Println("ℹ Unsupported block type")
			}
		case *notionapi.DividerBlock:
			fprintln(w, prefixes, "<!-- more -->\n")
		default:
			fmt.Println("ℹ Unimplemented block", b.GetType())
		}
	}
}
