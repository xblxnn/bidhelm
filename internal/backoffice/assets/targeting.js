for (const field of document.querySelectorAll("select[data-all]")) {
  const all = field.options[0];
  let wasAll = all.selected;
  field.addEventListener("change", () => {
    if (all.selected && !wasAll) {
      for (const option of field.options) option.selected = option === all;
    } else if (field.selectedOptions.length > 1) {
      all.selected = false;
    }
    if (!field.selectedOptions.length) all.selected = true;
    wasAll = all.selected;
  });
}
