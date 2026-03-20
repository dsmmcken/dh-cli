from deephaven import ui

@ui.component
def test_tag_group():
    tags, set_tags = ui.use_state(["React", "Python", "Java", "Go"])

    def on_remove(keys):
        set_tags([t for t in tags if t not in keys])

    return ui.flex(
        ui.tag_group(
            [ui.item(t, key=t) for t in tags],
            on_remove=on_remove,
            label="Languages",
        ),
        ui.text(f"Tags: {tags}"),
        ui.button("Reset", on_press=lambda: set_tags(["React", "Python", "Java", "Go"])),
        direction="column", gap="size-100",
    )

tag_group_widget = test_tag_group()
