from deephaven import ui

@ui.component
def test_links_nav():
    crumb_val, set_crumb_val = ui.use_state("home")

    return ui.flex(
        ui.link("Example Link", href="https://deephaven.io", target="_blank"),
        ui.breadcrumbs(
            ui.item("Home", key="home"),
            ui.item("Products", key="products"),
            ui.item("Detail", key="detail"),
            on_action=set_crumb_val,
        ),
        ui.text(f"Breadcrumb: {crumb_val}"),
        direction="column", gap="size-100",
    )

links_nav_widget = test_links_nav()
