from deephaven import ui

@ui.component
def test_markdown():
    return ui.flex(
        ui.markdown("# Hello Markdown\n\nThis is **bold** and *italic*.\n\n- Item 1\n- Item 2\n- Item 3\n\n```python\nprint('hello')\n```"),
        direction="column", gap="size-100",
    )

markdown_widget = test_markdown()
