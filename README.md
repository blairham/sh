# cla-signatures

Storage branch for the CLA Assistant action configured in
`.github/workflows/cla.yml` on `main`. It records who has signed
`CLA.md`, and carries no project code.

The action updates `signatures/version1/cla.json` and will not create
this branch itself — it fails with "Branch cla-signatures not found"
if the branch is missing, which is why it exists as an orphan with a
seeded empty signature list.

Do not protect this branch: the action has to push to it.
