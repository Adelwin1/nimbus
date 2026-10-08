import {render,screen,waitFor} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {beforeEach,expect,it,vi} from "vitest";
import RepositoryChecks from "@/components/journeys/RepositoryChecks";
const mocks=vi.hoisted(()=>({apiRequest:vi.fn()}));
vi.mock("@/lib/api",()=>({apiRequest:mocks.apiRequest}));
beforeEach(()=>{mocks.apiRequest.mockReset();});
it("queues a supported test profile for this journey run",async()=>{
 mocks.apiRequest.mockImplementation(async(_path:string,options:{method?:string})=>options.method==="POST"?{id:"check",status:"queued"}:{checks:[]});
 render(<RepositoryChecks runId="run-1" repository="owner/repo" commit={"a".repeat(40)}/>);
 const user=userEvent.setup();await user.selectOptions(screen.getByLabelText("Test profile"),"go-test");
 await user.clear(screen.getByLabelText("Repository directory"));await user.type(screen.getByLabelText("Repository directory"),"backend");
 await user.click(screen.getByRole("button",{name:"Run repository checks"}));
 await waitFor(()=>expect(mocks.apiRequest).toHaveBeenCalledWith("/github/runs/run-1/checks",expect.objectContaining({method:"POST",body:JSON.stringify({profile:"go-test",directory:"backend"})})));
});
it("links failing test evidence to the immutable commit without claiming causation",async()=>{
 mocks.apiRequest.mockResolvedValue({checks:[{id:"check",status:"failed",profile:"node-test",directory:".",result:{summary:"Repository tests failed.",findings:[{path:"test/login.test.cjs",line:12,kind:"test output location"}],related_files:[{path:"src/login.cjs",from:"test/login.test.cjs",relation:"static import; execution not proven"}],limitations:["No confirmed runtime call chain."]}}]});
 render(<RepositoryChecks runId="run-1" repository="owner/repo" commit={"a".repeat(40)}/>);
 expect(await screen.findByRole("link",{name:"test/login.test.cjs:12"})).toHaveAttribute("href",`https://github.com/owner/repo/blob/${"a".repeat(40)}/test/login.test.cjs#L12`);
 expect(screen.getByText(/static import; execution not proven/)).toBeVisible();
});
it("reports a queue failure without exposing the exception",async()=>{
 mocks.apiRequest.mockImplementation(async(_path:string,options:{method?:string})=>{if(options.method==="POST")throw new Error("private-token");return {checks:[]};});
 render(<RepositoryChecks runId="run-1" repository="owner/repo" commit={"a".repeat(40)}/>);
 await userEvent.setup().click(screen.getByRole("button",{name:"Run repository checks"}));
 expect(await screen.findByRole("alert")).toHaveTextContent("Check could not be queued");
 expect(screen.queryByText(/private-token/)).not.toBeInTheDocument();
});
