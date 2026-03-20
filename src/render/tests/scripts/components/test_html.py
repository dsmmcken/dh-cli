from deephaven import ui

# ui.html is a MODULE of raw HTML element factories, NOT a function.
# Usage: ui.html.div("text"), ui.html.h1("title"), ui.html.p("paragraph"), etc.
# Each creates a BaseElement("deephaven.ui.html.{tag}", *children, **attributes).

@ui.component
def test_html():
    count, set_count = ui.use_state(0)

    return ui.flex(
        # Heading via raw HTML h1 tag
        ui.html.h1("HTML Elements Test"),

        # Paragraph with inline formatting
        ui.html.p(
            "This paragraph has ",
            ui.html.b("bold"),
            " and ",
            ui.html.i("italic"),
            " text.",
        ),

        # Unordered list
        ui.html.ul(
            ui.html.li("First item"),
            ui.html.li("Second item"),
            ui.html.li("Third item"),
        ),

        # Div with nested structure
        ui.html.div(
            ui.html.span("Nested span inside div"),
        ),

        # Mix HTML elements with DH UI components
        ui.button(
            "Increment",
            on_press=lambda: set_count(count + 1),
            variant="primary",
        ),
        ui.html.p(f"Count: {count}"),

        # Horizontal rule
        ui.html.hr(),

        # Preformatted code block
        ui.html.pre(
            ui.html.code("x = 42"),
        ),

        direction="column", gap="size-100",
    )

html_widget = test_html()
