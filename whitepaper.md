Content Grid Protocol White Paper (v1.0)
summary
Scope note (current implementation): the network presently focuses on publisher registration and verifier-driven badge verification. Mining/task allocation/indexing features referenced below are legacy or roadmap items and are not active in the current codebase.
Content Grid is a decentralized content network and search engine protocol that aims to build a fairer, open and content-driven Internet. We use an innovative "Useful Proof of Work" mechanism to incentivize website publishers to contribute high-quality content and incentivize network nodes to provide crawling, computing and indexing services.

Unlike traditional search engines that rely on advertising revenue and opaque algorithms, Content Grid leverages blockchain technology, Full-Node Vector Indexing, and vector similarity search to create a censorship-resistant content discovery engine that is owned and maintained by the community. The protocol’s native token CONGRID is the core of the entire economic ecosystem and is used for staking, payment, rewards and governance to ensure that the interests of all participants are consistent with the long-term healthy development of the network.

1. Vision and Problems
1.1 Dilemma of the existing Internet
The current Internet content ecosystem is monopolized by a few centralized giants. This leads to several core questions:

Algorithmic black box: The ranking and visibility of content are determined by opaque commercial algorithms, making it difficult for creators to obtain fair exposure.
Censorship and Control: Centralized platforms have the power to unilaterally delete content or ban accounts, threatening free speech.
Inefficient value distribution: The advertising-driven model allows most of the value to be captured by the platform, and the income of content creators is severely squeezed.
Information silos: Content is locked within individual platforms, making cross-platform discovery and connection difficult.
1.2 Our solution: a decentralized content economy
Content Grid aims to solve these problems by:

Decentralized Indexing: Establish a globally shared content index database that is not controlled by any single entity.
Content-based discovery: Through advanced vector embedding (Embedding) technology, search based on content semantic similarity is realized instead of simple keyword matching.
Fair incentive mechanism: Reward all participants who contribute to the network, including content publishers and network node operators.
Useful work: Convert the computing power consumed by traditional blockchain mining into valuable work such as crawling, analyzing and indexing web pages.
2. System architecture
Content Grid adopts a layered architecture to decouple on-chain consensus from off-chain high-performance computing to achieve scalability and efficiency.

(This is a concept diagram placeholder, an actual diagram could depict the component interactions in more detail)

2.1 Blockchain Layer (Coordination Layer)
We built a sovereign application chain called content-grid-chain based on the Cosmos SDK. This chain is the "brain" and trust foundation of the entire system and is responsible for:

Identity and pledge management: Node operators register identities and obtain network permissions by staking CONGRID tokens. Website publishers need to register and verify the ownership of the **first-level domain** (Primary Domain). A first-level domain name can only be registered once.
Block Consensus: Adopts the standard Tendermint (CometBFT) BPoS consensus mechanism. This is the default solution of the Cosmos SDK, ensuring instant finality of transactions and high network security.
Task allocation: Use a deterministic algorithm based on block hash (Block Hash Based Assignment). The specific implementation is `Hash(BlockHash + TaskID + Counter) % TotalMiners`, which generates a pseudo-random number seed and deterministically selects execution nodes from the list of active miners. This approach takes advantage of the unpredictability of CometBFT without introducing additional VRF mechanisms.
Task result verification: Adopt majority verification based on on-chain consensus (On-Chain Majority Consensus). Miners submit the result hash to the chain, and CometBFT ensures the order of transactions. Legacy on-chain task logic (removed from current scope) automatically calculates the majority of submitted results (requires 67% Quorum). Only nodes that are consistent with the majority results can receive rewards. Nodes that do evil or make mistakes will be economically punished through the **Slashing** mechanism.
Economic model execution: Responsible for the reward distribution, transaction fee processing and governance voting of CONGRID tokens.

2.2 Full-Node Indexing Layer
Unlike traditional distributed hash table (DHT) sharded storage, Content Grid's long-term vision is a full-node indexing model: each worker node (miner) maintains a complete content index of the entire network.

Implementation-wise, "vector databases/ANN indexes" are pluggable (e.g. Chroma, FAISS, HNSWlib, etc.). To ensure efficiency and storage scalability, we tend to use shorter-dimensional vector representations and further derive shorter similarity fingerprints/signatures (e.g. 128-bit/256-bit) when needed for diversity constraints, deduplication and lightweight routing.

2.3 Off-chain execution and verification (Execution & Verification)
This is software run by independent worker nodes responsible for performing all compute-intensive tasks:

Web page crawling and Embedding: The node crawls the target web page, calculates the vector embedding, and stores it in the local index.
Similarity Query Service: When a publisher requests a snippet (containing a referral link), the network randomly selects a group of miners based on the current block hash. These miners locally query the 10 URLs most similar to the target content and return the results.
Verification mechanism: Nodes need to verify the publisher regularly.

In the current code implementation, a lightweight off-chain caching service `indexerd` is also provided: it automatically discovers publishers from the on-chain registry, periodically fetches the homepage and calculates embedding, and outputs a short signature at the same time. In this way, verifier/other components can obtain the cached results directly from `indexerd`, avoiding repeated access and repeated reasoning, while keeping large amounts of embedding off-chain.

3. Participant roles and incentives
3.1 Website Publishers (Content Providers)
How to participate:
Register its domain name, and the system will automatically identify the corresponding **first-level domain name** (such as `example.com`) and establish unique ownership to prevent the sub-domain name from being abused by others.
Embed an HTML snippet containing protocol verification information (including the `congrid.net` link and registrant wallet address) on the homepage of your website. At the same time, the snippet will also contain 10 links to similar content recommended by the network.
Get Incentive:
Availability rewards: As long as the website remains online and the links in the code snippets are verified by miners to be highly coincident with the network recommendation results (passing the coincidence threshold check), you can receive daily CONGRID token rewards.
Recommended traffic: Its website links will appear in HTML fragments of other websites with similar content, obtaining high-quality recommended traffic.
3.2 Node Operators (Network Workers/Miners)
How to participate:
Stake a large amount of CONGRID tokens to become an alternative worker node. The amount of pledge is the guarantee of its credibility and security.
Run the content-grid-d node software and local vector indexing service to maintain the entire network index.
Get Incentive:
Task rewards: Miners who are selected by the block hash algorithm and correctly complete the query task (returning the 10 most similar URLs) will receive rewards.
Validation rewards: Miners who correctly perform validation tasks (checking publisher code snippets) will also be rewarded.
Penalty mechanism: If the on-chain consensus arbitration determines that the task has not been completed correctly (such as the submission result is inconsistent with the majority), the node will be punished (Slashing).
Transaction fees: As a validator of the chain, you receive the fees generated by packaged transactions.
3.3 Consumers (Service Consumers)
How to participate:
Any person or organization in need of content discovery, data analysis or SEO services.
Get service:
Similarity Search: Free or paid access to the web for high-quality content similarity searches.
Post bounties: Pay CONGRID tokens to post customized tasks, such as:
Backlink Buying: Offer a bounty to add your own link to a website on a specific topic.
Crawl on demand: Pay a fee to have network nodes crawl and analyze any given website.
4. Tokenomics
CONGRID is the value carrier that drives the operation of the protocol. The following is the **current implementation caliber** and follow-up plans.

4.1 Token Utility (Utility)
- Staking (implemented): Verifiers participate in verifying distribution and income by staking `x/verifiers`.
- Payment (partially implemented): The chain already supports payment and settlement in slot/lease scenarios; "Consumer Bounty Tasks/Advanced API Payments" are still being planned and improved.
- Rewards (implemented): Publisher and validator rewards are implemented in round finalization of `x/registry`.
- Governance (partially implemented): The chain has the basic capabilities of the governance module, but the custom module parameter governance entrance is still being improved.

4.2 Supply and issuance pool (current default)
The total creation reference amount is set to 1 billion CONGRID (`ucongrid` is the smallest unit).

Current default issuance parameters:
- Operator’s retention: 40%
- Issuance pool: 60% (of which 10% are publishers and 50% are verifiers)
- Release duration: 100 years (linear release on an hourly basis)

In order to avoid "increasing the amount of rewards one by one each time", the rewards are changed to the pool transfer mode:
- The balance of the issuance pool is maintained by the tokenomics module;
- Each round the reward is transferred from the pool to the recipient;
- The unclaimed portion will be destroyed directly from the pool.

4.3 Distribution and allocation rules for each round (hour)
Under the default one-hour round (`round_interval_seconds=3600`):
- Publisher pool: ~114.155251 CONGRID/hour
- Validator pool: ~570.776255 CONGRID/hour

The general formula (in seconds for any round) is:
- `publisher_round = total_supply * publisher_bps * round_interval_seconds / (10000 * duration_hours * 3600)`
- `verifier_round = total_supply * verifier_bps * round_interval_seconds / (10000 * duration_hours * 3600)`

Allocation details:
- Publisher: The active publishers in the current round will be divided equally first; if the external links do not reach the threshold (`required_external_links_for_full_reward`), they will receive it in proportion; the unclaimed part will be destroyed.
- Validators: distributed only among validators who have passed and submitted successfully, with weight proportional to their pledges, and superimposed with the active publisher factor of their invitations (`stake × referral_factor`); destroyed when no one can claim it.

4.4 Value flow and deflation (current implementation)
- Supply side: Publisher/verifier rewards are released in rounds from the established distribution pool.
- Deflation side: Unclaimed publisher rewards, unavailable validator rewards and remaining balance will be destroyed.

illustrate:
- In the document, some of the capabilities such as "complete block-level inflation routing, full link for consumer rewards, and full link for fines and forfeitures" are still being implemented in stages; currently, the logic that has been launched shall prevail.

5. Roadmap
Phase 1: Protocol Core Implementation
[✓] Complete the content-grid-chain basic framework construction.
[✓] Implement node staking and registration modules (legacy mining scope).
[✓] Implement website registration and verification module (x/registry).
[✓] Implement task allocation, consensus and reward modules (legacy, removed from current scope).
Phase 2: Off-chain functions and testnet online
[ ] Develop off-chain task executor and full-node vector indexing service.
[ ] Release the internal test network and invite early node operators to participate.
Phase 3: Economic model and consumer functions come online
[ ] Launch the incentivized testnet and introduce publisher and token rewards.
[ ] Develop API gateway and consumer bounty task functions.
Phase Four: Mainnet Online and Community Governance
[ ] Completed the security audit and officially launched the main network.
[ ] Gradually transfer protocol governance rights to the CONGRID token holder community.
6. Conclusion
Content Grid is more than just a blockchain project, it is a social experiment to build a more open and fair next-generation content Internet. By tightly tying the interests of all participants to the healthy development of the network, we believe we can create a decentralized ecosystem that is vibrant, self-evolving, and brings real value to all users. We invite you to join us in building this future.
