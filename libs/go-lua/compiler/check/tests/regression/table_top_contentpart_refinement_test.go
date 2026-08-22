package regression

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Regression guard: type(x) == "table" followed by discriminant checks on
// dynamic JSON-shaped values should be sufficient to accumulate a typed
// content-part array without spurious return-type errors.
func TestTableTopContentPartRefinement(t *testing.T) {
	source := `
		type ImageSource = {
			type: string,
			url: string?,
			mime_type: string?,
			data: string?,
		}

		type ContentPart = {
			type: string,
			text: string?,
			source: ImageSource?,
		}

		local function image(url: string): ContentPart
			return { type = "image", source = { type = "url", url = url } }
		end

		local function collect_images(content: any): {ContentPart}
			local images = {}
			if type(content) == "table" then
				for _, img in ipairs(content) do
					if type(img) == "table" then
						if img.url then
							table.insert(images, image(img.url))
						elseif img.type == "image" and img.source then
							table.insert(images, img)
						end
					end
				end
			end
			return images
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		if result.Session != nil && result.Session.RootResult != nil && result.Session.RootResult.FlowInputs != nil {
			for _, ec := range result.Session.RootResult.FlowInputs.EdgeConditions {
				cs := ec.Condition.AllConstraints()
				if len(cs) == 0 {
					continue
				}
				t.Logf("edge %v -> %v: %v", ec.From, ec.To, cs)
			}
		}
		t.Fatalf("expected no errors, got: %v", fmt.Sprintf("%v", testutil.ErrorMessages(result.Diagnostics)))
	}
}
