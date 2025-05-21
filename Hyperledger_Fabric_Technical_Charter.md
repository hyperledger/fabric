**Technical Charter (the “Charter”)**
**for**
**Hyperledger Fabric a Series of LF Projects, LLC**

**Adopted May 21, 2025**

This Charter sets forth the responsibilities and procedures for technical contribution to, and oversight of, the Hyperledger Fabric ecosystem of open source projects, which has been established as Hyperledger Fabric a Series of LF Projects, LLC (the “Project”). LF Projects, LLC (“LF Projects”) is a Delaware series limited liability company. All contributors (including committers, maintainers, and other technical positions) and other participants in the Project (collectively, “Collaborators”) must comply with the terms of this Charter.

1.  **Mission and Scope of the Project**

    1.  The mission of the Project is to host and promote a vibrant ecosystem of different **Sub-Projects** related to Hyperledger Fabric. These **Sub-Projects** aim to provide solutions and components underpinned by a modular architecture delivering high degrees of confidentiality, resiliency, flexibility, and scalability, tailored to diverse use cases.

    2.  The scope of the Project includes collaborative development under the Project License (as defined herein) supporting the mission. This encompasses:

        1.  The development and maintenance of one or more distinct **Sub-Projects**.

        2.  Documentation, testing, integration, and the creation of other artifacts that aid the development, deployment, operation, or adoption of these **Sub-Projects** and the overall ecosystem.

        3.  Establishing and managing the governance framework for the ecosystem, including processes for recognizing new **Sub-Projects**.

2.  **Technical Steering Committee**

    1.  The Technical Steering Committee (the “TSC”), also referred to as the Ecosystem TSC, will be responsible for facilitating the governance and providing overarching technical coordination for the open source Project and its ecosystem of **Sub-Projects**. Its decisions on key ecosystem-wide matters are made according to the voting procedures in Section 4.

    2.  **TSC Composition:** The TSC voting members are initially those **Sub-Project Maintainers** from certain foundational **Sub-Projects**. Initial TSC voting members are those listed in the MAINTAINERS file in the `fabric` repository ([https://github.com/hyperledger/fabric/blob/main/MAINTAINERS.md](https://github.com/hyperledger/fabric/blob/main/MAINTAINERS.md)) and those in the `fabric-gateway` repository ([https://github.com/hyperledger/fabric-gateway/blob/main/MAINTAINERS.md](https://github.com/hyperledger/fabric-gateway/blob/main/MAINTAINERS.md)). Once a new **Sub-Project** is approved (as per Section 4) and its required repositories are created, the TSC voting membership can be expanded to include designated **Sub-Project Maintainers** from this new **Sub-Project**. The initial inclusion of maintainers from a newly recognized **Sub-Project** may be approved by an existing TSC member. For subsequent changes to a **Sub-Project's** representation on the TSC, for example adding all maintainers of a new repository within a Sub-Project to the TSC, a simple majority approval from the respective existing TSC from the specified Sub-Project is required. The TSC may also, by a decision made under the terms of Section 4, choose an alternative approach for determining the voting members of the TSC, and any such alternative approach (together with a list of then-current TSC voting members) will be documented in the MAINTAINERS files or a similar publicly accessible document. Any meetings of the Technical Steering Committee are intended to be open to the public, and can be conducted electronically, via teleconference, or in person.

    3.  **Roles within the Ecosystem:** The Project ecosystem generally will involve Contributors and **Sub-Project Maintainers**. The TSC may adopt or modify roles so long as the roles are documented by the TSC. Unless otherwise documented:

        1.  **Contributors** include anyone in the technical community that contributes code, documentation, or other technical artifacts to any recognized **Sub-Project** in the Project;

        2.  **Sub-Project Maintainers** are Contributors who have earned the ability to modify (“commit”) source code, documentation, or other technical artifacts in a specific recognized **Sub-Project’s** repository(ies) (details in Section 3); and

        3.  A Contributor may become a **Maintainer** for a particular **Sub-Project’s** repository by a simple majority approval of the existing **Maintainers** of that **Sub-Project’s** repository. Similarly, a **Maintainer** may be removed by a majority approval of the other existing **Maintainers** of that same Sub-Project’s respository.

    4.  Participation in the Project through becoming a Contributor and **Sub-Project Maintainer** is open to anyone so long as they abide by the terms of this Charter.

    5.  The TSC may (1) establish workflow procedures for the submission, approval, and closure/archiving of recognized **Sub-Projects** (subject to the voting rules in Section 4), (2) set ecosystem-wide minimum requirements or guidelines for the promotion of Contributors to **Sub-Project Maintainer** status (while allowing individual **Sub-Projects** to define more specific criteria), and (3) amend, adjust, refine and/or eliminate ecosystem-level roles, and create new roles, and publicly document any TSC roles and the roles and responsibilities of **Sub-Project Maintainers**, as it sees fit.

    6.  The TSC may elect a TSC Chair, who will preside over meetings of the TSC and will serve until their resignation or replacement by a TSC decision (as per Section 4). The TSC Chair, or any other TSC member so designated by the TSC, will serve as the primary communication contact between the Project and Hyperledger Foundation, a directed fund of The Linux Foundation.

    7.  **Responsibilities:** The TSC will be responsible for all aspects of oversight relating to the Project, which may include:

        1.  coordinating the technical direction of the Project;

        2.  approving project or system proposals (including, but not limited to, archiving and changing a sub-project’s scope);

        3.  organizing sub-projects and removing sub-projects;

        4.  creating sub-committees or working groups to focus on cross-project technical issues and requirements;

        5.  appointing representatives to work with other open source or open standards communities;

        6.  establishing community norms, workflows, issuing releases, and security issue reporting policies;

        7.  approving and implementing policies and processes for contributing (to be published in the CONTRIBUTING file) and coordinating with the series manager of the Project (as provided for in the Series Agreement, the “Series Manager”) to resolve matters or concerns that may arise as set forth in Section 7 of this Charter;

        8.  discussions, seeking consensus, and where necessary, voting on technical matters relating to the code base that affect multiple projects; and

        9.  coordinating any marketing, events, or communications regarding the Project.

3.  **Sub-Project Maintainers**

    1.  **Definition:** Each officially recognized **Sub-Project** within the ecosystem shall have its own group of **Sub-Project Maintainers** (hereinafter "Maintainers"). These Maintainers are the individuals who can be members of the TSC as per Section 2.2.

    2.  **Responsibilities:** These Maintainers are responsible for the day-to-day technical direction, codebase development and maintenance, release management (including defining and managing their own release cycles and any Long-Term Support (LTS) versions for their **Sub-Project**, in alignment with TSC guidelines established under Section 2), security vulnerability response, and community engagement for their particular **Sub-Project**. They shall ensure their **Sub-Project** operates in alignment with any TSC-defined guidelines.

    3.  **Autonomy:** Maintainer groups for specific **Sub-Projects** will have significant autonomy over their codebase and development roadmap, provided they operate within overarching TSC guidance.

    4.  **Selection and Removal:** The process for adding or removing Maintainers for a specific **Sub-Project** will be defined by that **Sub-Project's** community and its existing Maintainers, following established Hyperledger best practices for meritocracy and transparency, and subject to any minimum guidelines set by the TSC. New **Sub-Projects**, upon approval under Section 4, will propose their initial set of Maintainers.

4.  **Voting on Ecosystem-Wide Decisions**

    1.  **General Principle:** While the Project aims to operate by seeking consensus among Collaborators and relevant Maintainer groups, certain significant ecosystem-wide decisions require a formal voting process to ensure broad support and maintain the integrity of the Hyperledger Fabric ecosystem.

    2.  **Quorum for Sub-Project Maintainer Group Votes:** For a **Sub-Project's** Maintainer group to cast its collective vote on an ecosystem-wide matter, a simple majority of its **Sub-Project Maintainers** who are part of TSC must participate in its internal decision-making process, as defined by that group's procedures or, in absence thereof, by simple majority vote.

    3.  **Decision-Making on Significant Ecosystem-Wide Matters:** Decisions on significant ecosystem-wide matters shall require approval from a simple majority of the distinct Maintainer groups of currently recognized and active **Sub-Projects**. The process is as follows:

        1.  A proposal for an ecosystem-wide decision is brought before the TSC for discussion and refinement.

        2.  If the TSC deems the matter requires a formal vote under this section, it will be put to the Maintainer groups of all currently recognized and active **Sub-Projects**.

        3.  Each such **Sub-Project's** Maintainer group shall determine its collective approval or disapproval of the proposal via its own internal voting process (a simple majority vote of its active members).

        4.  Each **Sub-Project's** Maintainer group then signals its single collective decision (approve/disapprove) to the TSC.

        5.  The TSC tallies these group decisions. If the required simple majority of group approvals is met, the proposal is considered adopted by the ecosystem.

        6.  Matters requiring this vote include, but are not limited to:

            1.  Formal acceptance and recognition of a new **Sub-Project** into the ecosystem.

            2.  Archiving or sunsetting an existing recognized **Sub-Project**.

            3.  Making significant changes to any TSC-defined core principles or guidelines.

            4.  Adopting or significantly changing other ecosystem-wide policies.

    4.  Amendments to this Charter: Amendments to this Technical Charter (as per Section 10) shall require a **supermajority** vote of the distinct Sub-Project Maintainer groups.

    5.  Role of the TSC: The TSC is responsible for facilitating this voting process, formally defining criteria for a **Sub-Project** to be considered "active" and thus eligible to participate, setting the precise majority threshold, ratifying the outcomes of these votes, and documenting all such decisions.

    6.  Sub-Project Specific Matters Escalated to TSC: For matters primarily specific to a single **Sub-Project** that are escalated to the TSC for guidance or resolution, and do not constitute a significant ecosystem-wide decision as listed above, the TSC may provide recommendations or make decisions by a simple majority vote of TSC members present at a quorate meeting, or by a simple majority of all TSC members via electronic vote.

    7.  Dispute Resolution: In the event a vote under Section 4 cannot be resolved or leads to significant contention, any voting member of the TSC (representing their **Sub-Project**) may refer the matter to the Series Manager of LF Projects for assistance in reaching a resolution, in consultation with the Hyperledger Foundation.

5.  **Releases**

    1.  Each recognized **Sub-Project** shall manage its own release cadence, versioning scheme, and release artifacts, under the guidance of its **Sub-Project Maintainers**.

    2.  The TSC shall establish and maintain ecosystem-wide guidelines or minimum standards for releases, particularly concerning Long-Term Support (LTS) versions (support duration, scope), security patching coordination across **Sub-Projects** where applicable, and communication of release information to the broader community. Adherence to these guidelines will be expected for recognized **Sub-Projects** offering LTS.

6. **Compliance with Policies** 

    1. This Charter is subject to the Series Agreement for the Project and the Operating Agreement of LF Projects. Contributors will comply with the policies of LF Projects as may be adopted and amended by LF Projects, including, without limitation the policies listed at [https://lfprojects.org/policies/](https://lfprojects.org/policies/).  

    2. The TSC may adopt a code of conduct (“CoC”) for the Project, which is subject to approval by the Series Manager.  In the event that a Project-specific CoC has not been approved, the LF Projects Code of Conduct listed at [https://lfprojects.org/policies](https://lfprojects.org/policies) will apply for all Collaborators in the Project.

    3. When amending or adopting any policy applicable to the Project, LF Projects will publish such policy, as to be amended or adopted, on its web site at least 30 days prior to such policy taking effect; provided, however, that in the case of any amendment of the Trademark Policy or Terms of Use of LF Projects, any such amendment is effective upon publication on LF Project’s web site.

    4. All Collaborators must allow open participation from any individual or organization meeting the requirements for contributing under this Charter and any policies adopted for all Collaborators by the TSC, regardless of competitive interests. Put another way,

    5. The Project will operate in a transparent, open, collaborative, and ethical manner at all times. The output of all Project discussions, proposals, timelines, decisions, and status should be made open and easily visible to all. Any potential violations of this requirement should be reported immediately to the Series Manager.

7. **Community Assets**
 
    1. LF Projects will hold title to all trade or service marks used by the Project (“Project Trademarks”), whether based on common law or registered rights.  Project Trademarks will be transferred and assigned to LF Projects to hold on behalf of the Project. Any use of any Project Trademarks by Collaborators in the Project will be in accordance with the license from LF Projects and inure to the benefit of LF Projects.  

    2. The Project will, as permitted and in accordance with such license from LF Projects, develop and own all Project GitHub and social media accounts, and domain name registrations created by the Project community.

    3. Under no circumstances will LF Projects be expected or required to undertake any action on behalf of the Project that is inconsistent with the tax-exempt status or purpose, as applicable, of the Joint Development Foundation or LF Projects, LLC.

8. **General Rules and Operations.** 

    1. The Project will:

       1. engage in the work of the Project in a professional manner consistent with maintaining a cohesive community, while also maintaining the goodwill and esteem of LF Projects, Joint Development Foundation and other partner organizations in the open source community; and

       2. respect the rights of all trademark owners, including any branding and trademark usage guidelines.

9. **Intellectual Property Policy**

    1. Collaborators acknowledge that the copyright in all new contributions will be retained by the copyright holder as independent works of authorship and that no contributor or copyright holder will be required to assign copyrights to the Project. 

    2. Except as described in Section 7, all contributions to the Project are subject to the following: 

       1. All new inbound code contributions to the Project must be made using Apache License, Version 2.0 available at http://www.apache.org/licenses/LICENSE-2.0  (the “Project License”). 

       2. All new inbound code contributions must also be accompanied by a Developer Certificate of Origin ([http://developercertificate.org](http://developercertificate.org)) sign-off in the source code system that is submitted through a TSC-approved contribution process which will bind the authorized contributor and, if not self-employed, their employer to the applicable license;

       3. All outbound code will be made available under the Project License.

       4. Documentation will be received and made available by the Project under the Creative Commons Attribution 4.0 International License (available at [http://creativecommons.org/licenses/by/4.0/](http://creativecommons.org/licenses/by/4.0/)). 

       5. The Project may seek to integrate and contribute back to other open source projects (“Upstream Projects”). In such cases, the Project will conform to all license requirements of the Upstream Projects, including dependencies, leveraged by the Project.  Upstream Project code contributions not stored within the Project’s main code repository will comply with the contribution process and license terms for the applicable Upstream Project.

    3. The TSC may approve the use of an alternative license or licenses for inbound or outbound contributions on an exception basis. To request an exception, please describe the contribution, the alternative open source license(s), and the justification for using an alternative open source license for the Project. License exceptions must be approved by a two-thirds vote of the entire TSC. 

    4. Contributed files should contain license information, such as SPDX short form identifiers, indicating the open source license or licenses pertaining to the file.

10. **Amendments**

    1. This charter may be amended by following the voting rules specified in Section 4 and is subject to approval by LF Projects.
