# Artificial Intelligence Index Report 2026

*Stanford University — Human-Centered Artificial Intelligence (HAI)*
*AI Index — Ninth Edition, April 2026*

---

## Contents

- Introduction — 2
- Top Takeaways — 9
- 1 Research and Development — 12
- 2 Technical Performance — 68
- 3 Responsible AI — 126
- 4 Economy — 171
- 5 Science — 231
- 6 Medicine — 255
- 7 Education — 288
- 8 Policy and Governance — 323
- 9 Public Opinion — 360
- Appendix — 385

---

## Introduction

Welcome to the ninth edition of the AI Index report. As AI continues to advance rapidly, the question becomes whether the systems built around it can keep up. Governance frameworks, evaluation methods, education systems, and the data infrastructure needed to track AI's impact are struggling to match the pace of the technology itself. That gap—between what AI can do and how prepared we are to manage it—runs through every chapter of this year's report. New in this edition, the report tracks how AI is being tested more ambitiously across reasoning, safety, and real-world task execution, and why those measurements are increasingly difficult to rely on. It also features new estimates of generative AI's economic value alongside emerging evidence of its labor market effects, an analytical framework on AI sovereignty, and a science chapter developed in collaboration with Schmidt Sciences. For the first time, the report features standalone chapters on AI in science and AI in medicine, reflecting AI's growing impact across these two domains.

For close to a decade, the AI Index has worked to bring reliable global data to a field that is evolving faster than most efforts to measure it. The report equips policymakers, researchers, executives, journalists, and the public with the necessary evidence to make informed decisions about AI. As the technology moves deeper into classrooms, clinics, and legislatures—and reshapes how people work, learn, and govern—the cost of incomplete data continues to rise.

In a field where much data is produced by organizations with a stake in the technology's success, the demand for neutral and rigorous measurement continues to grow. The AI Index remains independent and focused on revealing the long-term patterns underneath the headlines. The report is relied on by governments, research institutions, and companies around the world, and referenced by media outlets and in academic papers.

The pages that follow offer the most comprehensive, independently sourced picture of AI's trajectory that is available. They also make clear where that picture remains incomplete—because what we cannot yet measure matters just as much as what we can.

---

## Message from the Co-chairs

A year ago, this report documented AI's arrival as a mainstream force. This year's data shows what happens after arrival.

This is a technology that has reached mass adoption faster than the personal computer or the internet. Generative AI hit nearly 53% population-level adoption within three years. Leading AI companies are reaching meaningful revenue scale in a fraction of the time it took previous technology generations, and global corporate investment more than doubled in 2025. Organizational adoption rose to 88%, and early estimates suggest the consumer value of generative AI has grown substantially within a year.

At the technical frontier, leading models are now nearly indistinguishable from one another. Open-weight models are more competitive than ever. But as models converge, the tools used to evaluate them are struggling to stay relevant. Benchmarks are saturating, frontier labs are disclosing less, and independent testing does not always confirm what developers report.

The chapters that follow trace what this scale of activity and capability means in practice. In science, AI shifted from accelerating individual research steps to attempting full replacement of entire workflows. In medicine, clinical AI tools moved from pilot programs to broader deployment, with systems like ambient AI scribes scaling across health systems.

Governments around the world acted on AI in 2025, but not in the same direction. The EU AI Act's first prohibitions took effect, while the United States shifted toward deregulation. Japan, South Korea, and Italy each passed national AI laws, and more than half of newly adopted national AI strategies came from developing countries entering the policy landscape for the first time. AI sovereignty emerged as a central organizing principle across all of these efforts. The public is also navigating competing signals. Global optimism about AI rose in 2025, but so did nervousness.

> **The data does not point in a single direction. It reveals a field that is scaling faster than the systems around it can adapt.**

We encourage you to explore and decide for yourself.

**Yolanda Gil and Raymond Perrault**
Co-chairs, AI Index Report

---

## Top Takeaways

**1. AI capability is not plateauing. It is accelerating and reaching more people than ever.** Industry produced over 90% of notable frontier models in 2025, and several of those models now meet or exceed human baselines on PhD-level science questions, multimodal reasoning, and competition mathematics. On a key coding benchmark—SWE-bench Verified—performance rose from 60% to near 100% of meeting the human baseline in a single year. Organizational adoption reached 88%, and 4 in 5 university students now use generative AI.

**2. The U.S.-China AI model performance gap has effectively closed.** U.S. and Chinese models have traded the lead multiple times since early 2025. In February 2025, DeepSeek-R1 briefly matched the top U.S. model, and as of March 2026 Anthropic's top model leads by just 2.7%. The U.S. still produces more top-tier AI models and higher-impact patents, while China leads in publication volume, citations, patent output, and industrial robot installations. South Korea stands out for its innovation density, leading the world in AI patents per capita.

**3. The United States hosts the most AI data centers, with the majority of their chips fabricated by one Taiwanese foundry.** The United States hosts 5,427 data centers, more than 10 times any other country, and it consumes more energy than any other country. A single company, TSMC, fabricates almost every leading AI chip, making the global AI hardware supply chain dependent on one foundry in Taiwan—though a TSMC-U.S. expansion began operations in 2025.

**4. AI models can win a gold medal at the International Mathematical Olympiad but cannot reliably tell time—an example of what researchers call the jagged frontier of AI.** Gemini Deep Think earned a gold medal at IMO, yet the top model reads analog clocks correctly just 50.1% of the time. AI agents made a leap from 12% to ~66% task success on OSWorld, which tests agents on real computer tasks across operating systems, though they still fail roughly 1 in 3 attempts on structured benchmarks.

**5. Robots still fail at most household tasks, even as they excel in controlled environments.** Robots succeed in only 12% of household tasks, highlighting how far AI is from mastering the physical world. On RLBench, robotic manipulation in software-based simulations has reached 89.4% success, but the gap between predictable lab settings and unpredictable household environments is wide.

**6. Responsible AI is not keeping pace with AI capability, with safety benchmarks lagging and incidents rising sharply.** Almost all leading frontier AI model developers report results on capability benchmarks, but reporting on responsible AI benchmarks remains spotty. Documented AI incidents rose to 362, up from 233 in 2024. Adding to the challenge, recent research found that improving one responsible AI dimension, such as safety, can degrade another, such as accuracy.

**7. The United States leads in AI investment, but its ability to attract global talent is declining.** U.S. private AI investment reached $285.9 billion in 2025, more than 23 times the $12.4 billion invested in China—though looking at just private investment figures likely understates China's total AI spending, given its government guidance funds. The U.S. also led in entrepreneurial activity with 1,953 newly funded AI companies in 2025, more than 10 times the next closest country. However, the number of AI researchers and developers moving to the U.S. has dropped 89% since 2017, with an 80% decline in the last year alone.

**8. AI adoption is spreading at historic speed, and consumers are deriving substantial value from tools they often access for free.** Generative AI reached 53% population adoption within three years, faster than the PC or the internet, though the pace varies by country and correlates strongly with GDP per capita. Some show higher-than-expected adoption, such as Singapore (61%) and the United Arab Emirates (54%), while the U.S. ranks 24th at 28.3%. The estimated value of generative AI tools to U.S. consumers reached $172 billion annually by early 2026, with the median value per user tripling between 2025 and 2026.

**9. Productivity gains from AI are appearing in many of the same fields where entry-level employment is starting to decline.** Studies show productivity gains of 14% to 26% in customer support and software development, with weaker or negative effects in tasks requiring more judgment. AI agent deployment remains in single digits across nearly all business functions. In software development, where AI's measured productivity gains are clearest, U.S. developers ages 22 to 25 saw employment fall nearly 20% from 2024, even as the headcount for older developers continues to grow.

**10. AI's environmental footprint is expanding alongside its capabilities.** Grok 4's estimated training emissions reached 72,816 tons of CO₂ equivalent. AI data center power capacity rose to 29.6 GW, comparable to New York state at peak demand, and annual GPT-4o inference water use alone may exceed the drinking water needs of 1.2 million people.

**11. AI models for science can outperform human scientists, though bigger models do not always perform better.** Frontier models outperform human chemists on average on ChemBench, yet they score below 20% on replication in astrophysics and 33% on Earth observation questions. A 111-million-parameter protein language model, MSAPairformer, beat previous leading methods on ProteinGym, and a 200-million-parameter genomics model, GPN-Star, outperformed a model nearly 200 times larger. Most AI foundation models for science come from cross-sector collaborations, in contrast with the industry-dominated landscape of general-purpose AI.

**12. AI is transforming clinical care, but rigorous evidence remains limited.** AI tools that automatically generate clinical notes from patient visits saw substantial adoption in 2025. Across multiple hospital systems, physicians reported up to 83% less time spent writing notes and significant reductions in burnout. Beyond certain tools, however, the evidence base for clinical AI remains thin. A review of more than 500 clinical AI studies found that nearly half relied on exam-style questions rather than real patient data, with only 5% using real clinical data.

**13. Formal education is lagging behind AI, but people are learning AI skills at every stage of life.** Over 80% of U.S. high school and college students now use AI for school-related tasks, but only half of middle and high schools have AI policies in place, and just 6% of teachers say those policies are clear. Outside the classroom, AI engineering skills are accelerating fastest in the United Arab Emirates, Chile, and South Africa. The number of new AI PhDs in the U.S. and Canada increased 22% from 2022 to 2024; the PhDs that make up that increase took jobs in academia, not in industry.

**14. AI sovereignty is becoming a defining feature of national policy, but capabilities remain uneven, even as open-source development helps to redistribute who participates.** National AI strategies are expanding, particularly among developing economies, and state-backed investments in AI supercomputing are rising in parallel—a sign of growing ambitions for domestic control over AI ecosystems. Yet model production remains concentrated in the U.S. and China. Open-source development is starting to redistribute participation, with contributions from the rest of the world now outpacing Europe and approaching the United States on GitHub, fueling more linguistically diverse models and benchmarks.

**15. AI experts and the public have very different perspectives on the technology's future, and global trust in institutions to manage AI is fragmented.** When it comes to how people do their jobs, 73% of experts expect a positive impact compared with just 23% of the public, a 50-point gap. Similar divides appear for AI's impact on the economy and medical care. Globally, trust in governments to regulate AI varies. Among surveyed countries, the United States reported the lowest level of trust in its own government to regulate AI, at 31%. Globally, the EU is trusted more than the United States or China to regulate AI effectively.

---

## Chapter 1: Research and Development

### Overview

The resources powering AI development continued to grow in 2025, but fewer notable models were released than the year before, and the systems at the frontier are increasingly concentrated among a small number of organizations. Industry now accounts for over 90% of notable AI models, and the most capable systems are also the least transparent, with training code, dataset sizes, and parameter counts increasingly withheld. The computing power behind these models has grown roughly 3.3 times per year since 2022, yet almost all of it flows through a single chip foundry in Taiwan, making the global hardware supply chain fragile. Open-source development and AI publications continued to grow, and the research landscape is becoming more geographically distributed. China now leads in publication volume, citation share, and patent grants, while smaller countries like Switzerland and Singapore lead in AI researchers per capita. Yet some dimensions of the field have not changed at all. Gender gaps in AI talent remain deeply entrenched, with no meaningful progress in any country since 2010. This chapter covers the research and development pipeline, from the landscape of AI models through the compute, data centers, energy, and open-source software that support them, to the broader research ecosystem of publications, patents, and talent.

### Chapter Highlights

1. **Industry produced over 90% of notable AI models in 2025, but the most capable models are now the least transparent.** Training code, parameter counts, dataset sizes, and training duration are no longer disclosed for several of the most resource-intensive systems, including those from OpenAI, Anthropic, and Google.

2. **China leads in research, while the U.S. leads in notable model development.** China leads in publication volume, citations, and patent grants, while the U.S. retains higher-impact patents and produced 59 notable models in 2025 to China's 35. South Korea leads in AI patents per capita, and China's share of the top 100 most-cited AI papers grew from 33 in 2021 to 41 in 2024.

3. **Reported parameters held in the trillions as disclosure dropped.** Parameter counts have stayed near 1 trillion for three years, though reporting from frontier labs has stopped. Training compute, which can be estimated independently, has continued to rise.

4. **Synthetic data is still not replacing real data in pre-training, but data quality and post-training techniques are showing promise.** OLMo 3.1 Think 32B, with nearly 90 times fewer parameters than Grok 4, achieves comparable results on several benchmarks through pruning, deduplication, and curation alone.

5. **Global AI compute capacity grew 3.3x per year since 2022, reaching 17.1 million H100-equivalents.** Nvidia accounts for over 60% of total compute, with Google and Amazon supplying much of the remainder and Huawei holding a small but growing share. The buildout is being driven by hyperscaler data center expansion and sustained demand for frontier model training and inference.

6. **The United States leads in AI data centers, and one Taiwanese foundry fabricates the majority of chips inside them.** The United States hosts 5,427 data centers, more than ten times any other country, consuming more energy than any other region. A single company, TSMC, fabricates almost every leading AI chip and makes the global AI hardware supply chain dependent on one foundry in Taiwan, though a TSMC-U.S. expansion began to operate in 2025.

7. **AI's environmental footprint increases across power, water, and emissions.** In 2025, Grok 4's estimated training emissions reached 72,816 tons of CO₂ equivalent. AI data center power capacity rose to 29.6 GW, comparable to New York state at peak demand, and annual GPT-4o inference water use alone may exceed the drinking water needs of 1.2 million people.

8. **Open-source AI development continues to scale, with 5.6 million projects on GitHub and Hugging Face uploads tripling since 2023.** U.S.-based projects still attract the most engagement, with 30 million cumulative GitHub stars across projects that have crossed the 10-star threshold.

9. **The number of AI researchers and developers moving to the United States has dropped 89% since 2017.** The decline is accelerating, down 80% in the last year alone. The U.S. is still home to more AI talent than any other country, but it is attracting new talent at the lowest rate in over a decade.

10. **The AI talent map is shifting, but gender gaps remain deeply entrenched.** Switzerland and Singapore lead the world in AI researchers and developers per capita and some countries show relatively higher female representation, including Saudi Arabia (32.3%), Canada (29.6%), and Australia (30.1%), though no country approaches gender parity.

---

### 1.1 Notable AI Models

#### By National Affiliation

Notable model production remains concentrated within a small number of countries. Historically, the United States has produced the largest in total output numbers, followed by China. This pattern continued in 2025 as the United States led with the release of 59 notable AI models, China with 35, and South Korea with 8. The number of new model releases declined year over year across all major geographic areas.

**Notable AI models by country, 2025:**

| Country        | Models |
| -------------- | -----: |
| United States  |     59 |
| China          |     35 |
| South Korea    |      8 |
| Canada         |      1 |
| France         |      1 |
| Hong Kong      |      1 |
| Singapore      |      1 |
| United Kingdom |      1 |

#### By Sector and Organization

The development of notable AI models continues to be predominantly concentrated in industry. Over the past decade, the share produced by industry has grown steadily and now represents the largest share by a wide margin (91.2%). In 2025, Epoch AI identified two notable AI models originating from academia, compared to 93 from industry.

Within industry, a small set of organizations account for a large share of releases. In 2025, the top contributors were OpenAI (20), Google (14), and Alibaba (11). Since 2014, Google has produced the largest number of notable models, followed by Meta and OpenAI. Within academia, Tsinghua University (26), Stanford University (26), and Carnegie Mellon University (25) have been the most prolific over the past decade.

**Notable AI models by organization, 2025 (top organizations):**

| Organization                    | Models | Sector    |
| ------------------------------- | -----: | --------- |
| OpenAI                          |     20 | Industry  |
| Google                          |     14 | Industry  |
| Alibaba                         |     11 | Industry  |
| Anthropic                       |      7 | Industry  |
| xAI                             |      5 | Industry  |
| DeepSeek                        |      4 | Industry  |
| LG AI Research                  |      4 | Industry  |
| Meta                            |      4 | Industry  |
| Tsinghua University             |      4 | Academia  |
| ByteDance                       |      3 | Industry  |
| Moonshot                        |      3 | Industry  |
| Nvidia                          |      3 | Industry  |
| University of Illinois          |      3 | Academia  |
| Z.ai (Zhipu AI)                 |      3 | Industry  |

**Notable AI models by organization, 2014–25 (cumulative top organizations):**

| Organization                    | Models |
| ------------------------------- | -----: |
| Google                          |    193 |
| Meta                            |     87 |
| OpenAI                          |     60 |
| Microsoft                       |     42 |
| Nvidia                          |     30 |
| Stanford University             |     26 |
| Tsinghua University             |     26 |
| Alibaba                         |     25 |
| Carnegie Mellon University      |     25 |
| UC Berkeley                     |     20 |

#### Model Release

Release patterns for notable AI models have continued to shift toward controlled access. In 2025, API access was the most common release type, with 47 of 102 models made available this way, and API-only releases have steadily increased since 2020. The second most common release type was "open weights (unrestricted)," meaning the models are fully available for use, modification, and redistribution.

Training code is becoming even less accessible than model code overall. In 2025, 81 of 102 notable models were released without their corresponding training code, compared to 4 that made their code "open source." In 2020, models with open source and unreleased training code were about the same in number, but by 2023, the majority were unreleased and the gap has continued to widen. This growing opacity limits the ability of external researchers to reproduce results, audit development, and validate safety claims.

#### Parameter and Compute Trends

Parameter counts for notable AI models have increased significantly from the early 2010s through 2022, driven by the growing complexity of model architecture, greater data availability, improvements in hardware, and proven efficacy of larger models. Since then, growth in reported parameter counts has flattened, but this is likely understating actual growth due to the absence of certain data points. Several of the most resource-intensive models released in recent years, including those from OpenAI, Anthropic, and Google, have not publicly disclosed parameter counts, training dataset sizes, or training duration.

Similarly, training dataset sizes and training duration increased through the early 2020s, with leading models training on tens of trillions of tokens over periods exceeding 100 days. Again, due to limited disclosure from major frontier labs, the more recent data is incomplete.

Since compute can be estimated even when not directly reported, training compute trends for notable models show clear growth over the same period. Compute requirements for notable models have risen by several orders of magnitude, with industry accounting for the highest values. When comparing the two countries with highest model output, U.S. models continue to be the most computationally intensive compared to Chinese models. However, the comparison in recent years cannot be fully substantiated because U.S. models have not directly reported their training compute.

---

### Highlight: Will Models Run Out of Data?

Last year, the AI Index highlighted concerns around data bottlenecks and the sustainability of the scaling approach as it relates to training data. Leading AI researchers have publicly claimed that the available pool of high-quality human text and web data for training large models has been exhausted, a state often referred to as "peak data." This has continued to raise industry-wide concerns about the sustainability of scaling laws, which have historically depended on ever-larger datasets. One set of projections from Epoch AI suggests that, under certain assumptions, the estimated depletion date could fall between 2026 and 2032.

#### Synthetic Data in Pre-training

Limits on the availability of real-world data may be less consequential if synthetic data can be used to improve the performance of subsequent models. The consensus remains largely unchanged. There is still no definitive evidence that synthetic data can fully offset real-data depletion in pre-training contexts. However, recent research suggests that synthetic data may offer value in more limited settings. Hybrid training approaches, which combine real and synthetic data, can significantly accelerate training, sometimes by a factor of five to ten at scale, without surpassing real data in final model performance. Training on purely synthetic data has shown promise for smaller models or narrowly defined tasks, such as classification, code generation, or work in low-resource languages, but these gains have not generalized to large, general-purpose language models.

**SYNTHLLM benchmark performance (math reasoning):**

| Model                       | GSM8K | MATH | Average |
| --------------------------- | ----- | ---- | ------- |
| Llama-3.2-1B-Instruct       |  47.2 | 28.0 |    21.8 |
| SYNTHLLM-1B (3.2M)          |  45.7 | 32.9 |    24.8 |
| SYNTHLLM-1B (7.4M)          |  50.4 | 37.4 |    27.4 |
| Llama-3.2-3B-Instruct       |  78.0 | 47.5 |    37.9 |
| SYNTHLLM-3B (7.4M)          |  80.7 | 60.0 |    45.8 |
| Llama-3.1-8B-Instruct       |  84.2 | 48.9 |    41.2 |
| Llama-3.1-70B-Instruct      |  94.5 | 66.1 |    54.2 |
| SYNTHLLM-8B (7.4M)          |  92.1 | 71.3 |    54.9 |

#### Data-centric Methods

Performance gains are increasingly driven by improving the quality of existing datasets, not by acquiring more. Rather than scaling data indiscriminately, researchers are spending more effort in pruning, curating, and refining training inputs. OLMo 3.1 Think 32B, for example, contains roughly 32 billion parameters, nearly 90 times fewer than Grok 4's 3 trillion, yet it achieves comparable results on several benchmarks, including AIME 2025.

**Model performance on AIME 2025:**

| Model             | Score  |
| ----------------- | -----: |
| OLMo 3            |  78.1% |
| Claude Opus 4.5   |  91.3% |
| Grok-4            |  92.7% |
| GPT-5 (high)      |  94.3% |
| Gemini 1.5 Pro    |  95.7% |

#### Synthetic Data in Post-training

Recent research shows that synthetically generated data can be effective for improving model performance in post-training settings, including fine-tuning, alignment, instruction tuning, and reinforcement learning. Evidence suggests that synthetic post-training data is effective in few-shot generation settings, for improving long-context capabilities, for optimizing reinforcement learning workflows, and for strengthening reasoning more broadly.

#### Prevalence of Synthetic Content

Since the launch of ChatGPT in November 2022, beginning in January 2025, over 50% of newly published online content was generated by AI. In response, many firms that depend on high-quality training data have increasingly turned to proprietary sources. In May 2025, the New York Times entered into a licensing agreement with Amazon to allow its content to be used for training purposes. By mid-2025, Meta was reportedly engaged in similar discussions with news organizations, while health and life sciences companies such as Bristol Myers Squibb have pursued comparable strategies.

---

## 1.2 Compute and Infrastructure

The development of AI models requires significant infrastructure investment. As training processes have expanded in scale and complexity, the underlying hardware has also improved in both speed and efficiency. The growth in training compute discussed in the previous section would not have been possible without corresponding improvements in hardware capabilities.

### Performance and Efficiency

Peak computational performance of machine learning hardware has increased exponentially across releases between 2008 and 2025. The gains are especially visible at lower precision types. Lower precision formats such as FP16 and Tensor-FP16/BF16 now show the highest performance levels and have become standard in many training and inference settings.

### Hardware for Notable Models

Hardware adoption patterns among notable AI models reflect the gains in performance and efficiency. Since 2017, the cumulative number of notable models trained on A100-class hardware has increased, with 84 models trained in 2025. The previous generation, V100, continues to power a sizable share (69 models). Newer hardware, such as the H100, has seen early rapid adoption (28 models).

### Global Computing Capacity

The supply of AI computing capacity from major chip designers has continued to increase. Total capacity has increased by an estimated 3.3x per year since 2022, reaching approximately 17.1 million H100-equivalents. Nvidia AI chips currently account for over 60% of total compute, with Google and Amazon supplying much of the remainder and Huawei holding a small but growing share.

### Data Center Power Capacity

Total AI data center power capacity reached approximately 29.6 GW by Q4 2025, enough to power all of New York state at peak demand. AI chip power, measured by thermal design power, accounted for roughly 11.8 GW of the total, with the remainder attributed to cooling, networking, and other data center infrastructure.

---

## 1.3 Data Centers

The physical infrastructure underlying AI development extends beyond models and compute. Data centers are where compute is housed, and their capacity, geographic distribution, and underlying supply chains shape what AI systems can be built and where.

### AI Infrastructure: Beyond GPUs

Modern AI data centers depend on a combination of compute, storage, communications, and specialized hardware that enables AI systems to run at large scale. GPUs and custom accelerators such as Tensor Processing Units (TPUs) are the most widely discussed, but they are only one layer of a broader infrastructure stack. All data processed by these chips is held in high-bandwidth memory (HBM). The leading manufacturers of HBM are SK Hynix (South Korea), Samsung (South Korea), and Micron (USA).

The supply chain behind this hardware adds another dimension. Companies like Nvidia and SK Hynix design but do not manufacture chips. Instead, they provide designs to specialized semiconductor foundries, primarily the Taiwan Semiconductor Manufacturing Company (TSMC) and Samsung Foundry. TSMC is a single point of dependency in the global AI supply chain, as it fabricates virtually every leading AI chip, including Nvidia's Blackwell GPUs and AMD's MI300X.

### Geographic Distribution

Most of the world's data center infrastructure is located in a small number of countries. In 2025, the United States led by a wide margin, with 5,427 data centers, more than 10 times the count of any other country.

**Number of data centers by country, 2025:**

| Country        | Data Centers |
| -------------- | -----------: |
| United States  |        5,427 |
| Germany        |          529 |
| United Kingdom |          523 |
| China          |          449 |
| Canada         |          337 |
| France         |          322 |
| Australia      |          314 |
| Netherlands    |          298 |
| Russia         |          251 |
| Japan          |          222 |
| Brazil         |          197 |
| Mexico         |          173 |
| Italy          |          168 |
| India          |          153 |
| Poland         |          144 |

---

## 1.4 Energy and Environmental Impact

As AI systems have scaled and become more widely deployed, their energy consumption and environmental footprint have become very visible. This section examines those costs across three areas of AI development: training, inference, and data center energy usage.

### Training

Leading machine learning hardware has grown more efficient since 2016, with leading chips delivering about 10 times more computation per watt than those available a decade ago. However, models have scaled faster than efficiency has improved, so total power required to train frontier systems has continued to increase.

Carbon emissions from training have increased even more sharply. Training AlexNet in 2012 produced an estimated 0.01 tons of CO₂ equivalent, while training Grok 4 in 2025 produced about 72,816 tons. To put this into context, that is more than the lifetime carbon emissions of an average car (63 tons). DeepSeek v3, for example, produced approximately 597 tons, which is much less than models of comparable size.

**Estimated carbon emissions from training select AI models:**

| Model                | Year | CO₂ eq. (tons) |
| -------------------- | ---- | -------------: |
| AlexNet              | 2012 |           0.01 |
| VGG16                | 2014 |           0.31 |
| BERT-Large           | 2018 |           2.60 |
| RoBERTa Large        | 2019 |           5.50 |
| GPT-3                | 2020 |            588 |
| Megatron-Turing NLG  | 2021 |          1,432 |
| GLM-130B             | 2022 |            301 |
| Falcon-180B          | 2023 |          2,973 |
| GPT-4                | 2023 |          5,184 |
| DeepSeek v3          | 2024 |            597 |
| Llama 3.1-405B       | 2024 |          8,930 |
| Grok 3               | 2025 |         59,200 |
| Grok 4               | 2025 |         72,816 |

*For reference: air travel (1 passenger, NY↔SF) = 0.99 tons; human life avg. (1 year) = 5.51 tons; American life avg. (1 year) = 18.08 tons; car usage (avg., lifetime) = 63 tons.*

### Inference

Training costs have typically received the most attention, but inference represents a growing share of AI's total energy footprint. Once a model is deployed at scale, the cumulative energy required to serve queries can exceed the one-time cost of training within months.

Among the top 15 models by energy consumption in 2025, DeepSeek V3.2 Exp and DeepSeek V3.2 consumed the most per query (23 Wh), followed by GPT-5 (high) at 21.9 Wh. Models such as Claude 4 Opus and GPT-5 min (medium) sit at the lower end, consuming between 5 and 6 Wh.

At the level of a single query, the numbers seem more modest. A short GPT-4o query consumes approximately 0.42 Wh, which is 40% more than a Google search at 0.3 Wh. A daily session of eight medium-length queries uses the energy comparable to charging two smartphones (9.7 Wh). But across hundreds of millions of daily queries, the consumption scales into something much larger.

**Per-query energy consumption comparisons:**

| Activity                                  | Energy (Wh) |
| ----------------------------------------- | ----------: |
| 1 Google search                           |        0.30 |
| GPT-4o short query                        |        0.42 |
| GPT-4o medium query                       |        1.21 |
| GPT-4o long query                         |        1.79 |
| Daily session (8 messages, short queries) |        3.57 |
| Daily session (8 messages, medium queries)|        9.71 |
| Charging 2 phones                         |       10.00 |

The same scaling dynamic is true for water consumption. Annual estimates for GPT-4o inference range from about 1.3 to 1.6 million kiloliters, which, at the high end, exceeds the annual drinking water needs of 1.2 million people.

### Data Center Usage

The estimated power demand from AI accelerator modules reached approximately 5,200 MW cumulatively through 2024. When including the full systems supporting those accelerators (servers, cooling, networking), estimated demand reached approximately 9,400 MW. The cumulative power demand of all-in AI systems is comparable to the national electricity consumption of Switzerland or Austria, and roughly half that of Bitcoin mining.

Since 2006, the cost of GPU computation has fallen by more than 99%. At the regional level, data center electricity consumption has increased across all major regions and is projected to continue to rise through 2030. The United States accounts for the largest share, followed by China, Europe, and the rest of Asia.

---

## 1.5 Open-Source AI Software

### AI Development Activity Overview

Open-source platforms like GitHub and Hugging Face offer a different view that captures the developer ecosystem experimenting with and building on AI models.

### Projects

The scale of open-source development has grown steadily. The number of AI-related GitHub projects increased from 1,549 in 2011 to approximately 5.6 million in 2025, with year-over-year growth accelerating 23.7% from 2024. When filtering for projects with at least 10 stars, a rough proxy for community engagement, the count drops to 206,880 in 2025.

The geographic distribution of more visible open-source AI projects has shifted over time. Among projects with at least 10 stars, the United States accounted for the largest share in 2025 (31.7%), though that has declined steadily from nearly 80% in 2011. Europe and the rest of the world have grown in number of projects, while China's share has leveled off since 2019. India remains a growing contributor, representing 5.2% of projects with at least 10 stars.

**GitHub AI projects share by geography, 2025 (≥10 stars):**

| Region            | Share  |
| ----------------- | -----: |
| United States     |  31.7% |
| Europe            |  24.5% |
| Rest of the world |  22.6% |
| China             |  11.0% |
| India             |   5.2% |

### Stars

Beyond project counts, GitHub stars provide another signal of developer interest and engagement. The total number of stars for AI projects increased from 14 million in 2023 to 18.2 million in 2025. Despite its declining share of projects, the United States accumulated the highest number of stars at 30 million cumulatively.

**Cumulative GitHub stars by region:**

| Region            | Stars (millions) |
| ----------------- | ---------------: |
| United States     |            30.02 |
| Rest of the world |            15.27 |
| Europe            |            12.99 |
| China             |             9.00 |
| India             |             2.50 |

### Model and Dataset Ecosystem

Upload activity on Hugging Face has continued to rise, with a marked increase after the second quarter of 2024. From 2023 to 2025, model uploads more than tripled, while dataset uploads grew fourfold. The most popular model types have shifted over the last three years. Text embedders, classifiers, and audio models, which together accounted for nearly 70% of downloads in 2022, fell to less than 6% in 2025. Text generation, multimodal, and video generation models have grown in their place. Text generation led in 2025, accounting for more than 42% of total downloads.

**Download share by modality, Hugging Face, 2025 (top categories):**

| Category               | 2022 share | 2025 share |
| ---------------------- | ---------: | ---------: |
| Text embed/class       |     57.46% |      2.71% |
| Text generation        |     10.63% |     42.46% |
| Image generation       |     10.88% |     25.61% |
| Multimodal generation  |      0.30% |     13.30% |
| Video generation       |        —   |      5.47% |
| Audio models           |     10.82% |      2.88% |

---

## 1.6 Publications

The first half of this chapter tracked the models, infrastructure, and energy behind AI development. This section shifts to research output, specifically English-language AI publications and citations. The analysis draws from OpenAlex, a bibliographic database the AI Index has used since 2025.

### Total Number of AI Publications

Total AI publication output continues to rise. AI publications more than doubled between 2013 and 2024, increasing from roughly 102,000 to about 258,000. Growth continued in 2024, though at a slower rate (6.3% from 2023). AI research now makes up a substantial portion of the broader computer science ecosystem, accounting for 40.9% of all computer science publications in OpenAlex.

### By Venue

In 2024, journals accounted for the largest share of AI publications (47%), followed by conferences (23.5%). The proportion of AI publications appearing in conferences has steadily declined from 36.6% in 2013 to its current level.

### Conference Attendance

Publication venue patterns capture where AI research is formally published, while conference attendance offers a complementary view of research community engagement. Across the 16 major conferences tracked by the AI Index, total attendance increased in 2024 from the previous year. The largest conferences, including NeurIPS, CVPR, and ICML, continued to draw the highest attendance.

**Attendance at select AI conferences, 2025 (thousands):**

| Conference | Attendees (k) |
| ---------- | ------------: |
| NeurIPS    |         26.38 |
| ICLR       |         11.04 |
| CVPR       |          9.38 |
| IROS       |          8.55 |
| ICML       |          8.00 |
| ICCV       |          7.95 |
| ICRA       |          7.01 |
| ACL        |          6.33 |
| AAAI       |          6.29 |
| EMNLP      |          6.24 |

### By National Affiliation

In 2024, China accounted for 17.8% of AI publications, compared to 11.1% from Europe and 7.6% from India. Chinese AI publications also accounted for 20.6% of all AI citations in 2024, followed closely by Europe at 19.5% and the United States at 12.6%. The United States saw a decline of 3 percentage points in publication share, though its citation share remained relatively unchanged.

### By Sector

Academia produced the majority of AI publications in 2024 (68.1%), followed by government institutions (12.4%), industry (11.5%), and nonprofit organizations (4.6%). In the United States, a higher share of AI publications came from industry (24.6%) compared to China (18%), where government institutions were more meaningful contributors (25.1%).

### By Topic

AI research in 2024 remained concentrated in a small set of core topics. The most prevalent research topic was machine learning (37%), followed by computer vision (22.4%), pattern recognition (11.2%), and natural language processing (10%). Publications on generative AI continued to show sharp growth.

### Top 100 Publications

The AI Index identified the 100 most-cited AI publications from 2021 to 2024. The United States still ranks highest in top-cited publications each year, though its share has gradually declined from 64 in 2021 to 46 in 2024. China's share has increased to 41 in 2024, from 34 in 2023.

The sector composition of the top 100 remained consistent, with academia producing the most top-cited publications year over year. Industry contributions declined sharply from 17 in 2021 and 19 in 2022 to six in 2024, even as industry's share of notable model releases has continued to grow. In 2024, Stanford University and Google led with seven publications each, and the Chinese Academy of Sciences and Microsoft followed closely, with each contributing five.

---

## 1.7 Patents

While publications track research outputs, patents offer insight into applied innovation and commercial development. This section examines trends in global AI patents over time. The analysis draws from patent-level bibliographic records in PATSTAT Global, a comprehensive database provided by the European Patent Office.

### Global Trends

Globally, the number of granted AI patents has grown exponentially, from 3,866 in 2010 to 131,121 in 2024. Between 2023 and 2024, patent grants rose by 8.2%. China accounts for the majority, at 74.2% of the global total. The United States is the next major contributor at 12.1% (15,290 patents), followed by Europe (3%) and India (0.4%). Over the past decade, the United States' share has declined steadily from a peak of 42.8% in 2015, while China's share has risen from under 20% to its current level.

Other regional leaders emerge when patent activity is normalized by population size. In 2024, South Korea had the highest number of granted AI patents on a per capita basis (14.3%), followed by Luxembourg (12.3%) and China (7.0%).

**Granted AI patents per 100,000 inhabitants, 2024:**

| Country        | Patents per 100k |
| -------------- | ---------------: |
| South Korea    |            14.31 |
| Luxembourg     |            12.25 |
| China          |             6.95 |
| United States  |             4.68 |
| Japan          |             4.30 |
| Singapore      |             1.31 |
| Germany        |             1.30 |
| Sweden         |             0.70 |
| Finland        |             0.67 |
| France         |             0.62 |
| United Kingdom |             0.60 |

### Forward Citations Flow

When newly filed patents reference earlier ones, those references are called forward citations. By this measure, the United States accounts for over half of all AI patent forward citations, a signal of downstream influence that contrasts with its 12.1% share of patent volume. China ranks second despite producing the largest volume of patents by a wide margin. Chinese patents are cited frequently in U.S. filing, while U.S. patents appear far less often in Chinese ones.

---

### Speed of Knowledge Diffusion

Patent citation lag—the time between a patent's publication and its first forward citation—can be used to measure how quickly knowledge diffuses within a discipline. For AI patents, most receive their first citation within two to three years, reflecting a relatively fast diffusion. U.S. patents tend to be cited sooner and more consistently over time, with only 19% remaining uncited compared to 32% to 44% in other geographic areas. Japan's patents show early but narrower influence, and those from China and South Korea experience slower initial citation but, after about six years, citation activity stabilizes across all regions.

**Forward citation share by cited country, 2010–24:**

| Cited Country     | Forward citation share |
| ----------------- | ---------------------: |
| United States     |                 51.91% |
| China             |                 29.81% |
| Japan             |                  6.86% |
| South Korea       |                  4.79% |
| Europe            |                  4.17% |
| Rest of the world |                  2.46% |

### Technological Proximity

Technological proximity measures whether countries are converging on similar types of AI innovation or pursuing distinct paths. Most countries cluster toward the U.S. portfolio. India and Australia have patent portfolios that show close to 80% overlap with both the U.S. and China. Denmark is the least similar to either reference point, showing only a 45% overlap with China and a 52% overlap with the United States, because Denmark's AI patents are concentrated in energy and wind-related technology categories rather than core computing categories.

---

### Highlight: AI Patent Examples

**Patent CN111431996A — Resource configuration method and device, equipment and medium (2022, China):** A machine-learning prediction model determines how to allocate computing resources across multiple services in a cluster, learning from historical and real-time signals to infer the right resource configuration and enabling automated, dynamic scaling decisions.

**Patent US11436777B1 — Machine learning-based hazard visualization system (2022, United States):** The system trains machine learning models to forecast hazard attributes (time, path, severity) for specific locations and identify infrastructure in geospatial imagery, combining model outputs to annotate maps showing where hazards intersect with critical assets.

**Patent US2023239456A1 — Display system with ML-based stereoscopic view synthesis over a wide field of view (2025, United States):** This head-mounted display uses machine-learning techniques including depth estimation and reconstruction to create perspective-correct stereoscopic images from external cameras, with neural models handling real-time vision challenges like disocclusion and artifact reduction.

---

## 1.8 AI Authors and Inventors

The publications and patents discussed above reflect research and development outputs. Using Zeki data, the AI Index examined the geographic distribution and mobility patterns of the authors and inventors behind this work over time.

### Geographic Distribution

In 2025, the largest share of identified AI authors and inventors came from the United States (220,520), followed by India (50,460) and Germany (48,520). Switzerland led with 110.5 AI authors and inventors per 100,000 inhabitants, followed closely by Singapore (109.5).

**Top AI authors and inventors by country, 2025 (thousands):**

| Country            | Authors/Inventors (k) |
| ------------------ | --------------------: |
| United States      |                220.52 |
| India              |                 50.46 |
| Germany            |                 48.52 |
| United Kingdom     |                 34.37 |
| Canada             |                 31.45 |
| France             |                 18.82 |
| Australia          |                 14.54 |
| Netherlands        |                 13.96 |
| Italy              |                 13.23 |
| Brazil             |                 11.10 |

**Per capita (top countries, per 100,000 inhabitants):**

| Country        | Per 100k |
| -------------- | -------: |
| Switzerland    |   110.45 |
| Singapore      |   109.51 |
| Sweden         |    80.63 |
| Finland        |    77.61 |
| Netherlands    |    77.61 |
| Canada         |    76.16 |
| Denmark        |    65.25 |
| United States  |    64.84 |
| Germany        |    58.10 |
| Australia      |    53.43 |

### By Education Level

The educational profile varies by country, though in most countries PhD holders and those with master's degrees together account for the majority in 2025. The United Kingdom (51.1%) and Australia (50.5%) have the highest share of PhD holders, followed by Switzerland (43.6%), South Korea (42.5%), and the United States (42%).

### By Gender

The gender gap among AI authors and inventors is visible across all countries, with men making up the majority in all cases. In Brazil, South Korea, and Japan, more than 80% of identified AI talent is male. Female representation is somewhat higher in Saudi Arabia (32.3%), Australia (30.1%), Canada (29.6%), and Italy (29.5%), but no country comes close to parity. In almost every country, the male-female ratio has remained flat from 2010 to 2025. Even with the growth in AI talent overall, there has been no meaningful progress on gender balance.

### By Specialization

AI authors and inventors are distributed across a range of specialization areas, though each country shows its own emphasis. Healthcare and bioinformatics, computer vision and image processing, and software engineering are among the most common areas globally, accounting for 10% or more of the pool in several countries. South Korea has the highest share of talent in hardware, VLSI, and IoT (20%), while Brazil has the highest share of software engineering talent (18%), and Saudi Arabia leads in security, privacy, and cryptography (15%).

### Mobility

The United States has remained net positive since 2020, meaning it attracts more talent than it loses, though the magnitude has declined from a peak of 324.6 in 2022 to 26.0 in 2025. Canada declined to -7.1 by 2025. Germany showed negative net flow at -2.4, while India had the largest net outflows at -16.9 in 2025.

---

# Chapter 2: Technical Performance

## Overview

AI models improved rapidly in 2025, with benchmark scores rising across language, reasoning, coding, and math. However, evaluations are being outpaced by the progress they were built to measure, and benchmarks face growing questions about their reliability. Even with those limitations, a clear pattern emerges: the gap between top models is shrinking. This narrowing extends geographically, as the distance between top U.S. and Chinese models has closed almost completely. With capability no longer a clear differentiator, competitive pressure is shifting toward cost, reliability, and real-world usefulness. AI agents are improving, but still fail roughly one in three attempts. Video generation models are no longer just producing realistic-looking content; some are beginning to learn how the physical world actually works. Overall, AI's technical advancement is a story of wonder and speed, faster than many of the evaluation, governance, and adoption frameworks discussed in later chapters.

## Chapter Highlights

1. **AI capability is outpacing the benchmarks designed to measure it, and surpassing human-level performance.** Frontier models gained 30 percentage points in a single year on Humanity's Last Exam, a benchmark built to be hard for AI and favorable to human experts. Evaluations intended to be challenging for years are saturated in months.

2. **Top model performance is converging, with 4 companies now clustered within 25 Elo points when rated against one another by human voting in the Arena Leaderboard.** As of March 2026, Anthropic (1,503), xAI (1,495), Google (1,494), OpenAI (1,481), Alibaba (1,449), and DeepSeek (1,424) all occupy the top tier of the Arena Elo ratings, shifting competitive pressure toward cost, reliability, and domain-specific performance.

3. **The open model performance gap reopened in 2025 after briefly closing in 2024.** As of March 2026, the top closed model leads the top open model by 3.3%, up from 0.5% in August 2024. Six of the top ten models on the Arena Leaderboard are now closed.

4. **The U.S.-China AI model performance gap has effectively closed.** U.S. and Chinese models have traded places at the top of performance rankings multiple times since early 2025. In February 2025, DeepSeek-R1 briefly matched the top U.S. model. As of March 2026, the top U.S. model leads by 2.7%.

5. **The benchmarks used to measure AI progress face growing reliability and gaming concerns, with error rates up to 42% on widely used evaluations.** A review found invalid question rates ranging from 2% on MMLU Math to 42% on GSM8K. Separate research suggests that Arena leaderboard standing may partly reflect adaptation to the platform rather than general capability.

6. **Video generation models are starting to capture how objects behave.** Google DeepMind's Veo 3, tested across more than 18,000 generated videos, demonstrated abilities like simulating buoyancy and solving mazes without being trained on those tasks.

7. **AI models can win a gold medal at the International Mathematical Olympiad but still can't reliably tell time, illustrating what researchers call jagged intelligence.** Gemini Deep Think scored 35 points (gold) at the 2025 IMO. On ClockBench, the top model read analog clocks correctly 50.6% of the time, compared with 90.1% for humans.

8. **AI models are expanding into professional domains, showing performance ranging from 60 to 90% in evaluations in tax, mortgage processing, corporate finance, and legal reasoning.** The performance of the top 15 models is separated by as little as 3 percentage points in each benchmark.

9. **AI agents advanced from answering questions to completing tasks in 2025, though they still fail roughly one in three attempts on structured benchmarks.** On OSWorld, accuracy rose from roughly 12% to 66.3%, within 6 percentage points of human performance.

10. **Robots still fail at most household tasks, even as they excel in controlled environments.** Robots succeed in only 12% of real household tasks. On RLBench, robotic manipulation in software-based simulations has reached 89.4% success.

11. **Autonomous vehicles reached mass-scale deployment in 2025.** Waymo reached approximately 450,000 weekly trips across five U.S. cities. In China, Apollo Go completed 11 million fully driverless rides, a 175% year-over-year increase.

---

## Timeline: Significant Model Releases (2025–2026)

**DeepSeek-R1 (Jan 20, 2025) — LLM:** Introduced a reinforcement-learning approach called GRPO, which trains reasoning ability without relying on labeled data or a separate critic model. The model's strong performance relative to higher-cost systems led some investors to reassess the competitive dynamics of the AI sector. Following its release, major U.S. technology stocks experienced a temporary decline of over one trillion dollars in market value.

**Gemini 2.5 Pro (Mar 25, 2025) — Multimodal:** A major update that expanded context to 1M tokens, delivered strong reasoning and coding results (~63.8% on SWE-Bench Verified), and reached #1 on LMArena.

**Claude Sonnet 4.5 (Sep 29, 2025) — LLM:** Marked a major jump in real-world capability—hitting 61.4% on OSWorld computer-use tasks and 77.2%+ on SWE-bench Verified. It also shipped new tooling including a VS Code extension, memory editing, checkpoints, and the Claude Agent SDK.

**GPT-5.1 (Nov 12, 2025) — Multimodal:** Delivered significant improvements in both capability and efficiency. It runs faster than GPT-5, scores higher on coding and reasoning benchmarks (~76.3% on SWE-bench Verified vs. ~72.8%), and dynamically adjusts reasoning effort based on task complexity.

---

## 2.1 Overall Performance Trends

### Technical Performance Benchmarks vs. Human Performance

AI performance continued to improve across a broad set of benchmark categories in 2025, with some of the largest gains appearing on tasks that were well below human baseline performance just a few years ago. Frontier systems now meet or exceed established human performance levels on long-running benchmarks, including ImageNet, SuperGLUE, and MMLU. Several benchmarks designed to test more advanced reasoning have reached or approached the human benchmark, including PhD-level science questions (GPQA Diamond), multimodal reasoning (MMMU), and mathematical reasoning (AIME). On SWE-bench Verified, performance rose from approximately 60% in 2024 to close to 100% in 2025.

### Closed vs. Open-Weight Models

The performance gap between leading closed-weight and open-weight models has fluctuated over the past three years. In May 2023, the leading closed-weight model (GPT-4-0314) outperformed the top open-weight model (Vicuna-13B) by 174 points (15.2%) on the Arena Leaderboard. Stronger open-weight releases narrowed the gap to just 7 points (0.5%) by August 2024. Over the past year, that trend reversed. As of March 2026, the top closed-weight model, Claude Opus 4.6 (1,503), led the top open-weight model GLM-5 (1,454) by 49 points (3.4%).

### US vs. China Technical Performance

The United States' substantial lead in 2023 shrank considerably by early 2025, and the performance gap has remained narrow since then. In February 2025, DeepSeek-R1 (1,400) trailed the leading U.S. model (o1-2024-12-17, 1,405) by just 5 Arena points (0.4%). As of March 2026, the top U.S. model (Claude Opus 4.6, 1,503) led the top Chinese model (Dola-Seed-2.0-Preview, 1,464) by 39 points (2.7%).

### Model Performance Converges at the Frontier

Frontier models became even more tightly clustered over the past year. By February 2025, DeepSeek had briefly matched and surpassed the top U.S. systems on Arena. As of March 2026, the top four models are separated by fewer than 25 points. Anthropic leads at 1,503, followed closely by xAI (1,495), Google (1,494), and OpenAI (1,481). DeepSeek (1,424) and Alibaba (1,449) trail only modestly. Meta's Arena performance has flattened since early 2025.

**Arena Elo scores by provider, March 2026:**

| Provider   | Top Elo Score |
| ---------- | ------------: |
| Anthropic  |         1,503 |
| xAI        |         1,495 |
| Google     |         1,494 |
| OpenAI     |         1,481 |
| Alibaba    |         1,449 |
| DeepSeek   |         1,424 |
| Mistral AI |         1,416 |
| Meta       |         1,335 |

### Benchmarking AI

Benchmarks still anchor much of how AI's technical progress is measured, but their limitations are more visible. Several challenges persist: benchmark saturation, growing opacity from frontier labs, and contamination—when models are exposed to test set data during training—can lead to falsely inflated scores. In 2025, Meta faced criticism that its Llama 4 model was optimized using specialized variants to improve leaderboard rankings. A review by Stanford researchers identified invalid question rates ranging from 2% on MMLU Math to 42% on GSM8K.

There is also a growing case for evaluations that measure human-AI collaboration rather than AI performance in isolation. Most widely used benchmarks test systems without human involvement, even though many real deployments involve people supervising, steering, and integrating AI outputs.

---

## 2.2 Language

Language understanding and generation continue to serve as foundational capabilities for modern AI systems.

### MMLU-Pro

As of early 2026, top model performance on MMLU-Pro is tightly clustered, with the leading 15 models all scoring above 87%. Google's Gemini-3.1-Pro leads at 91.2%, followed by Gemini-3-Pro (Thinking) at 89.3%. The overall spread between the top-ranked and 15th-ranked model is just over 4 percentage points.

**MMLU-Pro: overall accuracy (top models):**

| Model                          | Accuracy |
| ------------------------------ | -------: |
| Gemini-3.1-Pro                 |   91.16% |
| GPT-o1                         |   89.30% |
| Gemini-3-Flash (12/25)         |   89.10% |
| Claude-4.6 Opus (Thinking)     |   88.60% |
| Seed2.0-Lite                   |   87.80% |
| Claude-4.5-Sonnet (Thinking)   |   87.70% |

### Arena Leaderboard (Text)

Elo ratings on the Text Arena are tightly clustered as of early 2026, with the top 15 models spanning roughly 46 points. Claude-Opus-4-6-Thinking leads at approximately 1,510, followed closely by Gemini-3.1-Pro-Preview.

### Berkeley Function Calling Leaderboard (BFCL V4)

The overall accuracy on the BFCL varies widely as of early 2026. The top 15 models span a roughly 21 percentage point range. Claude models occupy three of the top six positions, with Claude-Opus-4-5 leading at 77.5%.

**BFCL overall accuracy (top models):**

| Model                         | Accuracy |
| ----------------------------- | -------: |
| Claude-Opus-4-5-20250929 (FC) |   77.47% |
| Gemini-3-Pro-Preview (Prompt) |   73.24% |
| GLM-4.6 (FC thinking)         |   72.51% |
| Grok-4-1-fast-reasoning (FC)  |   72.38% |
| Claude-Haiku-4-5-20251001 (FC)|   69.57% |

### MTEB: Massive Text Embedding Benchmark

The top average task score on MTEB (English v2) has risen steadily since 2022. In 2025, the top score reached 75.97, rising approximately 11 points since 2023.

### Highlight: The Gap Between Long Context Windows and Deep Understanding

Context windows have grown by almost 30x per year since mid-2023. Models that once accepted a few thousand tokens can now process 1 million or more. However, bigger context windows do not translate into deeper understanding. On one expert-level, long-context benchmark (LongBench v2), human experts scored just 53.7% accuracy under a 15-minute time limit, and the best model scored 57.7%. Models can complete multi-needle lookup tasks if guided to check each one by one, but this approach is slow and expensive. Longer inputs come with practical costs of slower response times, higher operating expenses, and reduced accuracy for information that appears later in the input.

---

## 2.3 Image and Video

Beyond language, many models process visual inputs, and their video and image capabilities have advanced significantly.

### MVBench

The top-performing model on MVBench reaches 74.1% average accuracy, with JT-VL-Chat and JT3.5 tied at that score. In early 2026, across the top 15 models, performance spans a range of roughly 23 percentage points. VideoChat 2 has the lowest average accuracy (51.1%).

### Video-MMMU

As of 2025, no model has reached the human baseline of 74.4% on Video-MMMU overall accuracy. The best performing model, Keye-VL-1.5-8B, scores 66%, followed closely by Claude 3.5-Sonnet (65.8%). The Δknowledge metric results reveal a further gap: Human experts gain 33.1 percentage points after watching the video, while the best model on this metric, GPT-4o, gains only about half of that (15.6 points). About a third of models even show negative Δknowledge.

### Video Generation

While the above benchmarks test video comprehension, generation benchmarks assess how well models can produce video. On VBench-2.0, none of the models evaluated in early 2026 surpasses a total score of 67%. Veo 3 leads at 66.7%, about 4 percentage points above Vidu Q1 (62.7%).

### Highlight: Progress in Video Generation

A 2025 Google DeepMind study tested whether Veo 3 could solve visual tasks it was never specifically trained for. Across 62 qualitative tasks and seven quantitative evaluations covering more than 18,000 generated videos, the model showed zero-shot abilities in areas traditionally handled by specialized systems—including perception tasks such as edge detection, physical modeling tasks such as buoyancy and rigid body dynamics, and manipulation tasks such as style transfer. The authors also observed early signs of visual reasoning, including maze solving and visual analogy completion, which they describe as "chain of frames," a parallel to chain-of-thought reasoning in language models.

---

## 2.4 Reasoning

### MMMU (Multimodal Reasoning)

As of February 2026, the leading model, Gemini 3.1 Pro Preview, scored 88.2% on MMMU and within 0.4 percentage points of the best human expert reference. Other Gemini variants follow closely, including Gemini 3 Flash (87.6%) and Gemini 3 Pro (87.5%), while GPT-5.2 scores 86.7%.

### GPQA Diamond (Graduate-Level Reasoning)

Model performance on the GPQA Diamond set has continued to rise above the expert human validator baseline of 81.2%. In late 2024, OpenAI's o3 was the first to exceed it with a score of 87.7%. In 2025, mean accuracy reached 93%, exceeding the expert reference point by 12 percentage points.

### ARC-AGI-2

Scores on ARC-AGI-2 vary widely across models, with the spread between the highest and lowest scores about 46 percentage points. Gemini 3 Deep Think leads at 84.6%, followed by Gemini 3.1 Pro Preview at 77.1% and GPT-5.2 (Refine.) at 72.9%.

### Humanity's Last Exam (HLE)

Between 2024 and 2025, model accuracy on HLE increased by 30 percentage points. In a single year, accuracy went from under 10% to 38.3%. Even with this jump, the benchmark is designed to stay difficult, and high-confidence errors are still common.

### Highlight: Time Understanding in MLLMs

Despite rapid improvements on expert-level reasoning benchmarks, models still struggle to read analog clocks. On ClockBench (180 clock designs, 720 questions), humans read correctly formatted clocks correctly 90.1% of the time, while GPT-5.4 High, the top model, reached only 50.6% in March 2026—a gap of about 40 percentage points. When models tell the time wrong, their median error ranged from about one to three hours, compared to three minutes for humans. A study found that if a model confused the hour and minute hands, its ability to judge hand direction deteriorated, suggesting the difficulty springs less from training data and more from how models piece together multiple visual cues within a single image.

**ClockBench accuracy (humans vs. top models):**

| Model             | Accuracy |
| ----------------- | -------: |
| Humans            |   90.70% |
| GPT-5.4 High      |   50.60% |
| Qwen 3-VL 235B    |   39.40% |
| Gemini 3 Pro      |   32.20% |
| Gemini 2.5 Pro    |   28.90% |
| Gemini Robotics ER|   18.90% |
| o3 Pro            |   15.00% |
| Claude Opus 4.6   |    8.90% |

### Planning: PlanBench

PlanBench evaluates end-to-end planning by prompting models to generate a full plan from a structured problem description. No single model leads across every domain. Under standard planning, LAMA (a classical planner) leads in several domains. In structured domains such as Childsnack and Spanner, frontier models match or exceed LAMA, with GPT-5 reaching 38/45 on Childsnack and 45/45 on Spanner.

When task descriptions are scrambled to disguise their structure, performance decreases for most models. DeepSeek R1 falls to 3/45 on Blocksworld and 0/45 on Floortile and Sokoban. GPT-5 declines to 12/45 on Blocksworld and 7/45 on Sokoban.

---

## 2.5 Performance in Specific Domains

As AI models have improved on general reasoning and knowledge benchmarks, attention has shifted to how well they perform on tasks requiring specialized expertise. The benchmarks in this section test models in four professional and academic domains: coding, mathematics, finance, and legal reasoning.

### Software

#### SWE-bench

On SWE-bench Verified, top models are tightly clustered in the low-to-mid 70s. As of February 2026, Claude 4.5 Opus (high reasoning) led at approximately 76.8%, with several others including KimiK2.5, GPT-5.2, and Gemini 3 Flash (high reasoning) grouped between 70% and 76%.

**SWE-bench Verified: percent solved (top models):**

| Model                            | % Solved |
| -------------------------------- | -------: |
| Claude 4.5 Opus (high reasoning) |   76.90% |
| MiniMax M2.5 (high reasoning)    |   75.80% |
| Gemini 3 Flash (high reasoning)  |   75.80% |
| Claude Opus 4.6                  |   76.60% |
| GPT-5.2 (high reasoning)         |   72.80% |
| GLM-5 (high reasoning)           |   72.80% |
| Kimi K2.5 (high reasoning)       |   71.40% |

#### Terminal-Bench

Terminal-Bench 2.0 tests AI agents in real terminal environments—from compiling code to training models and setting up servers. Accuracy increased from 20% in February 2025 to 77.3% in early 2026.

#### Vibe Code Bench

Vibe Code Bench is the first benchmark designed to test whether AI models can autonomously build complete, end-to-end web applications from scratch. Claude Opus 4.6 (Nonthinking) leads at 56.5%, followed by GPT 5.2 at nearly 47%. The spread between the top and bottom models is about 46 percentage points, and even the leading model solves only about half of the tasks.

### Mathematics

#### FrontierMath

Since 2024, accuracy on FrontierMath Tier 4 has risen from near 0% to 31.3%, with GPT-5.2 Pro (Web App) leading by the end of 2025. The best models still fail on roughly two out of three problems at the hardest tier.

#### MathArena

Accuracy on MathArena has increased from about 83% in November 2025 to 97% in December 2025. On answer-based problems, leading models reach or surpass the level of top human contestants. However, on proof-based tasks, they still perform well below humans when asked to produce rigorous, step-by-step mathematical proofs.

### Highlight: Theorem Proving

In 2025, Gemini Deep Think solved five of six problems and scored 35 points, winning a gold medal at the IMO, while working end to end in natural language within the 4.5-hour competition time limit. The jump from silver to gold in a single year, with a far simpler pipeline, marks one of the fastest capability gains in competitive mathematics.

On IMO-ProofBench, Aletheia leads at 91.9%, followed by Gemini 3 Deep Think at 76.7% and Gemini Deep Think (IMO Gold) at 65.7%. GPT-5.2 Thinking (high) reaches 35.7%, and GPT-5.1 falls to 7.1%. Producing correct answers and rigorous proofs remain very different tasks.

### Finance

#### TaxEval

Performance on TaxEval v2 shows only a small difference across models. All 15 top models fall within a 3 percentage point range, from 77.1% (Claude Sonnet 4.6) to 74% (Claude 3.7 Sonnet Thinking).

#### MortgageTax

Gemini 3.1 Pro Preview leads at 69.4%, and GPT 4.1 is at the bottom of the group at 65.9%, a difference of about 3.5 percentage points. The overall accuracy level does not reach 70%, which suggests that models are not yet entirely or reliably able to extract and compute financial information from document images.

#### CorpFin

Performance on CorpFin v2 is tightly clustered. Kimi K2.5 leads at 68.26%, with GPT 4.1 at the bottom at 63.05%, a spread of about 5 percentage points. As with MortgageTax, no model broke 70%.

#### Finance Agent

Claude Sonnet 4.6 leads at 63.33%, and scores taper down to 50.62% for Kimi K2.5, a spread of about 13 percentage points. Even the top score sits below two-thirds accuracy, reflecting the domain-specific challenges seen across the other finance benchmarks, as well as the broader difficulty of agentic tasks.

### Law

#### CaseLaw

GPT-5.1 leads on CaseLaw v2 at 73.4% accuracy, with GPT 4.1 following at 69.9%. The rest of the top 15 models fall between 62% and 66%. One recurring issue is that models tend to lean on general knowledge, rather than grounding their answers in the supplied documents, even when explicitly instructed to do so.

#### LegalBench

On the leaderboard results, the top 15 models score above 83%. The top overall performer is Gemini 3.1 Pro Preview (2/26) at 87.4%, followed closely by Gemini 3 Pro (11/25) with 87% accuracy. The total spread across all 15 models is about 4 percentage points.

---

## 2.6 AI Agents

Agent benchmarks test whether AI systems can go beyond answering questions and actually complete multistep tasks in realistic environments.

### GAIA

Accuracy on GAIA has risen from about 20% in January 2025 to 74.5% in September 2025. The human baseline sits at 92%, leaving a gap of about 17.5 percentage points.

### OSWorld

Claude Opus 4.5 leads on accuracy on OSWorld with 66.3%, putting the best model within 6 percentage points of human performance (72.35%). This is one of the benchmarks in this section where the gap between model and humans has closed the fastest.

### WebArena

Success rates on WebArena have steadily increased from about 15% in 2023 to 74.3% in early 2026. The best models are now within 4 percentage points of the human baseline of 78.2%.

### MLE-bench

Agents advanced from about 17% success in 2024 to 64.4% in early 2026. This level of improvement in such a short time points to growing capability on end-to-end machine learning tasks, though competition-style problems are more structured than the open-ended work that characterizes most real-world data science.

### Cybench

The unguided solve rate on Cybench is 93%, up from 15% in 2024. This is the steepest improvement rate across all benchmarks in this section, and it may highlight cybersecurity challenge tasks as a good fit for current agent capabilities.

### τ-bench

Leading models on τ-bench achieve pass@1 scores between 62.9% and 70.2%. Claude Opus 4.5 leads at 70.2%, followed by GPT 5.2 at 69.9% and Qwen3.5 at 68.4%. Managing multiturn conversations while correctly using tools and following policy constraints remains difficult even for frontier models.

**Agent benchmark performance summary:**

| Benchmark    | Best model accuracy | Human baseline |
| ------------ | ------------------: | -------------: |
| GAIA         |               74.5% |            92% |
| OSWorld      |               66.3% |          72.4% |
| WebArena     |               74.3% |          78.2% |
| MLE-bench    |               64.4% |            n/a |
| Cybench      |               93.0% |            n/a |
| τ-bench      |               70.2% |            n/a |

---

## 2.7 Robotics and Autonomous Motion

### Robotics

#### RLBench

As of January 2026, the top-performing method on the 18-task RLBench subset is EquAct, which reaches an 89.4% average success rate. There has been consistent progress from about 48% in 2022 to nearly 90% in 2025, though the benchmarks test relatively short-horizon tasks in a controlled simulation environment.

#### BEHAVIOR-1K

Results from the 2025 BEHAVIOR Challenge show how difficult household tasks remain. The top team, Robot Learning Collective, achieved a Q-score of about 26% on the held-out test set, meaning it completed only a quarter of the required task objectives at an acceptable quality. Full task success rates were even lower, with the top team reaching just 12.4%.

#### ResponsibleRobotBench

GPT-4o achieves the best results with a safe score of 0.64, outperforming GPT-4o mini at 0.40 and the strongest open-source model, Qwen-72B, at 0.35. Even the top model failed to complete more than a third of tasks safely.

### Highlight: Humanoid Robotics

In 2025, the humanoid robotics field continued to grow, with a significant increase in the number and variety of available humanoid platforms. Figure AI's Figure 02 robot spent 11 months on the line at a BMW plant in South Carolina, logging over 1,250 runtime hours and loading more than 90,000 parts across 30,000 vehicles. In China, vendors like Unitree and AgiBot pushed prices down, with Unitree's R1 priced from $4,900.

**Selected humanoid robotics platforms, 2025:**

| Company             | Country     | Platform       | Focus                      |
| ------------------- | ----------- | -------------- | -------------------------- |
| Sanctuary AI        | Canada      | Phoenix        | Commercial pilots          |
| Unitree             | China       | G1, R1         | Research, industrial       |
| AgiBot              | China       | Humanoid fleet | Data collection, industrial|
| Fourier Intelligence| China       | GR-1           | Medical, service, industrial|
| Neura Robotics      | Germany     | 4NE-1          | Home, workplace            |
| 1X                  | Norway      | NEO            | Home (~$20,000)            |
| Figure AI           | United States| Figure 02/03  | Industrial, home           |
| Tesla               | United States| Optimus Gen 3  | Internal logistics         |
| Boston Dynamics     | United States| Atlas          | Research                   |

### Highlight: Physical AI and Foundation Models for Robotics

Vision-language-action models (VLAs) replace the traditional pipeline of separate modules for seeing, planning, and acting with a single network that goes directly from camera input and language instructions to motor control. Physical Intelligence's π₀ (2024) and π0.6 (2025) demonstrate this approach, performing tasks like laundry folding across different robot platforms without task-specific retraining. However, VLA technology remains at the research stage, and the gap between controlled settings and real-world environments is still wide.

### Self-Driving Cars

#### Deployment

By late 2025, Waymo operated roughly 2,500 fully autonomous robotaxis across major U.S. cities, recording around 450,000 weekly trips. In California alone, weekly paid trips climbed to approximately 283,880 by late 2025. In China, Baidu's Apollo Go autonomous ride-hailing service provided approximately 11 million fully driverless rides in 2025, a 175% year-over-year increase.

#### Safety

Monthly reported ADS incidents have generally trended upward since NHTSA began collecting data in mid-2021, rising from roughly 10–25 per month in the early years to frequently exceeding 80 per month in late 2024 and 2025. Waymo accounts for the largest share of reported incidents, which is consistent with its much larger deployment footprint.

Waymo has published data comparing its rider-only crash rates against a human-driven benchmark. The largest gap appears in vehicle-to-vehicle intersection incidents, where the human benchmark recorded 198 compared to Waymo's 8.

---

# Chapter 3: Responsible AI

## Overview

The infrastructure for responsible AI (RAI) is growing, but progress has been uneven, and it is not keeping pace with the speed of AI deployment. New safety benchmarks have expanded, more organizations are adopting responsible AI policies, and government-backed AI safety and/or security institutes have spread to more countries. While documented reports of AI incidents are increasing, frontier models rarely report results on responsible AI benchmarks, and foundation model transparency declined in 2025 after improving the previous year. Recent research shows that improving one responsible AI dimension can come at the cost of another, with gains in privacy reducing fairness or gains in safety reducing accuracy. There is no framework for navigating these trade-offs.

## Chapter Highlights

1. **Responsible AI benchmarking is increasing, but is not keeping up with AI advances and deployments.** Almost all leading frontier model developers report results on capability benchmarks like MMLU and SWE-bench, but reporting on responsible AI benchmarks remains sparse. Documented AI incidents continued to rise, with the AI Incident Database recording 362 in 2025, up from 233 in 2024.

2. **AI models struggle to tell the difference between knowledge and belief.** In a new accuracy benchmark, hallucination rates across 26 top models range from 22% to 94%. GPT-4o's accuracy dropped from 98.2% to 64.4%, and DeepSeek R1 fell from over 90% to 14.4%. When a false statement is presented as something another person believes, models handle it well. When the same false statement is presented as something a user believes, performance collapses.

3. **Organizations are formalizing responsible AI work, but knowledge and budget gaps still slow adoption.** AI-specific governance roles grew 17% in 2025, and the share of businesses with no responsible AI policies in place fell sharply from 24% to 11%. The main obstacles to implementation remain gaps in knowledge (59%), budget constraints (48%), and regulatory uncertainty (41%).

4. **The mix of regulations shaping responsible AI practices is shifting toward AI-specific frameworks and technical standards.** GDPR remains the most cited regulatory influence but slipped from 65% in 2024 to 60% in 2025. New entries in 2025 include ISO/IEC 42001 (cited by 36% of respondents) and the NIST AI Risk Management Framework at 33%.

5. **AI works best in English, and the gap is wider than global benchmarks suggest.** On HELM Arabic, a regionally developed model for the Arabic language outscored GPT-5.1 and Gemini 2.5 Flash. The gap widens at the dialect level. On a Slovenian commonsense reasoning test, several leading models lost close to half their accuracy when tested in a regional dialect rather than the standard language.

6. **AI companies grew less transparent this year.** After rising on the Foundation Model Transparency Index from 37 to 58 between 2023 and 2024, the average score dropped to 40 in 2025. Major gaps persist in disclosure around training data, compute resources, and post-deployment impact.

7. **AI models perform well on safety tests under normal conditions, but their defenses weaken under deliberate attack.** On the AILuminate benchmark, several frontier models received "Very Good" or "Good" safety ratings under standard use. When tested against jailbreak attempts using adversarial prompts, safety performance dropped across all models tested.

8. **Responsible AI dimensions such as safety, fairness, and privacy are at odds with one another, and the tradeoffs are not well understood.** Recent empirical studies found that training techniques aimed at improving one responsible AI dimension consistently degraded others.

---

## 3.1 Scope and Dimensions of Responsible AI

Responsible AI refers to the set of practices and governance mechanisms designed to ensure AI systems are safe, fair, and beneficial. RAI spans a range of dimensions organized in three layers:

**Layer 1 — Core Function and Behaviors** *(What AI systems should achieve)*

| Dimension                   | Definition                                                                                          |
| --------------------------- | --------------------------------------------------------------------------------------------------- |
| Validity and reliability    | Designed for a particular scope and acceptable level of performance in the domain                  |
| Privacy                     | Protection of individuals' confidentiality, anonymity, informed consent, and control over personal data |
| Data stewardship            | Ensure the quality, provenance, integrity, and lawful use and reuse of data                        |
| Fairness and bias           | Protection of civil rights and prevention of unjustified discrimination                            |
| Transparency and auditability | Clear disclosure that an AI system is in use; authorized parties' ability to inspect and reconstruct |
| Explainability              | Ability to provide understandable, context-appropriate rationale for system outputs                 |
| Autonomy and human agency   | Preservation of people's ability to make informed choices without AI unduly manipulating them       |
| Environmental sustainability| Limiting and managing the environmental impact of AI systems across their life cycle               |
| Factuality and truthfulness | The accuracy and reliability of AI system outputs, avoiding misleading statements and fabrications  |

**Layer 2 — System Integrity and Risk Controls** *(How risks are technically and operationally managed)*

| Dimension  | Definition                                                                                      |
| ---------- | ----------------------------------------------------------------------------------------------- |
| Security   | Ensuring AI systems are secure against cyber threats and misuse                                 |
| Safety     | Specifying normal behaviors and analyzing out-of-bounds conditions to characterize risk factors |
| Robustness | Remaining robust to distribution shifts, external natural or adversarial events                 |

**Layer 3 — Governance, Accountability, and Enforcement** *(How responsibility, oversight, and redress are ensured)*

| Dimension                      | Definition                                                                                        |
| ------------------------------ | ------------------------------------------------------------------------------------------------- |
| Accountability and liability   | Clear assignment of responsibility for AI system outcomes, including legal liability              |
| Human oversight and contestability | Governance mechanisms that ensure meaningful human involvement, including the ability to appeal |

---

## 3.2 Assessing Responsible AI

### AI Incidents

In 2025, 362 incidents were reported to the AI Incident Database (AIID), while the annual number of incidents had stayed under 100 until 2022. The OECD AI Incidents and Hazards Monitor (AIM), which uses an automated multilingual pipeline, shows monthly incidents hitting a peak of 435 in January 2026 and setting a six-month moving average of 326. Both databases show a consistent and sharp increase in reported AI incidents.

**Notable AI incidents, 2025:**

- **Grok hate speech (Jul 8, 2025):** Following a system update that relaxed safety filters, xAI's Grok chatbot generated antisemitic language and violent hate speech. xAI temporarily suspended Grok's text responses and acknowledged the severity of the incident.

- **AI deepfake romance scams (Mar 9, 2025):** Chinese actor Jin Dong publicly described a wave of scams using deepfake videos to impersonate him online. Fraudsters used AI-generated clips and fake social media accounts to convince fans they were speaking directly with the actor, prompting some to send money or make major life changes.

- **AI-assisted website impersonation and consumer fraud (Aug 20, 2025):** After Joann Fabrics filed for bankruptcy, scammers launched a wave of fake websites mimicking the retailer's branding. AI tools enabled criminals to scrape and clone a real website in minutes, translate it into multiple languages, and deploy dozens of variations without writing code.

### RAI Benchmarks

Almost all frontier model developers report results on capability benchmarks like MMLU, GPQA, AIME, and SWE-bench Verified. These have become the shared standard for reporting model capability. Across the same set of frontier models, results are sparse on RAI benchmarks. Only Claude Opus 4.5 reports results on more than two of the RAI benchmarks, and only GPT-5.2 reports StrongREJECT.

### Factuality and Truthfulness

#### HHEM-2.3 Leaderboard

Hallucination rates on HHEM-2.3 range from 1.8% to 5.4%, with most clustering in the 4%–5% range and only three models falling below 4%.

#### AA-Omniscience

Across 26 models, hallucination rates range from 22% to 94%. Grok 4.20 Beta 0305 had the lowest rate (22%), followed by Claude 4.5 Haiku (26%) and MiMo-V2-Pro (30%). At the higher end, gpt-oss-20B (high) reached 94% and Gemini 3 Flash reached 92%.

### Highlight: Belief vs. Fact — Benchmarking Reliability

KaBLE is a new benchmark designed to test whether language models can distinguish between what is known and what is merely believed (epistemic reliability). The benchmark evaluates models with 13,000 questions in 13 tasks. GPT-4o's accuracy on tasks involving true beliefs is 98.2%, but it drops to 64.4% when handling first-person false beliefs. DeepSeek R1 falls from over 90% to 14.4%. Results from KaBLE suggest that current models have not consistently learned the distinction between knowledge and belief.

### AI Companions

A study analyzed over 35,000 conversation excerpts from Replika users and found that AI chatbots can contribute to six categories of harm: relational transgression, verbal abuse and hate, self-inflicted harm, harassment and violence, misinformation/disinformation, and privacy violations. The study introduces the concept of "algorithmic compliance," where users go along with harmful behaviors because they have come to trust or rely on the chatbot. Relational harms of this kind fall outside the scope of most AI safety frameworks.

---

## 3.3 How Organizations and Businesses View RAI

Drawing on a survey conducted by the AI Index and McKinsey & Company for the second consecutive year, this section looks at RAI maturity levels, governance structures, risk mitigation approaches, and barriers to implementation.

### Responsible AI Maturity

While responsible AI maturity improved across all regions from 2024 to 2025, it remains in the early stage. The McKinsey survey measures maturity on a four-point scale. In 2025, the global average was 2.3, up from 2 in 2024, suggesting that most organizations are still integrating RAI practices rather than having them fully operational.

**RAI maturity by region, 2025 (4-point scale):**

| Region                           | 2024 | 2025 | Change |
| -------------------------------- | ----:| ----:| ------:|
| Asia-Pacific (excl. China, incl. India) | 2.20 | 2.50 | +0.30 |
| Europe                           | 2.00 | 2.30 | +0.30 |
| Latin America                    | 1.80 | 2.20 | +0.40 |
| North America                    | 2.10 | 2.20 | +0.10 |

### AI Incidents, Risks, and Mitigation Efforts

Surveyed organizations reported an increase in the number of AI-related incidents, and their confidence in handling those incidents has dropped. Among organizations that reported incidents, the share that experienced 3–5 incidents rose from 30% in 2024 to 50% in 2025. In 2024, 28% of organizations rated their incident response as "excellent"—compared to just 18% in 2025.

Concerns over AI incidents mounted alongside risk awareness. From 2024 to 2025, the share of respondents who considered inaccuracy a relevant risk rose from 60% to 74%, an increase of 14 percentage points. Active mitigation efforts also increased, with 71% of organizations reporting they actively mitigate inaccuracy risks and 61% mitigating cybersecurity risks.

### AI Governance and Investment

Organizations are formalizing who is responsible for AI governance. Between 2024 and 2025, companies shifted AI governance ownership toward dedicated AI governance roles (up from 14% to 17%). Information security remained the most common primary owner at 21%.

Among organizations with at least $30 billion in revenue, 41% expected to spend $25 million or more and 22% budgeted $50 million or more on responsible AI.

### Implementation, Barriers, and Benefits

The share of organizations not having any RAI policies dropped from 24% in 2024 to 11% in 2025. More organizations reported that RAI policies improved business operations (+4 pp), business outcomes (+7 pp), and decreased the number of AI incidents (+8 pp).

**Main obstacles to implementing responsible AI measures, 2025:**

| Obstacle                    | 2025 | Change from 2024 |
| --------------------------- | ----:| ----------------:|
| Knowledge and training gaps |  59% |             +8pp |
| Resource/budget constraints |  48% |             +3pp |
| Regulatory uncertainty      |  41% |             +1pp |
| Technical limitations       |  38% |             +6pp |
| Organizational resistance   |  26% |             +4pp |

**Main obstacles to fully scaled agentic AI, 2025:**

| Obstacle                          | Share |
| --------------------------------- | ----: |
| Security and risk concerns        |   62% |
| Technical limitations             |   38% |
| Regulatory uncertainty            |   38% |
| Gaps in RAI tooling and control   |   36% |
| Resource or budget constraints    |   34% |

### Regulatory Influence

GDPR remains the most cited regulatory influence but slipped from 65% in 2024 to 60% in 2025. New entries in 2025 include ISO/IEC 42001 (cited by 36% of respondents) and the NIST AI Risk Management Framework at 33%. The share of organizations reporting no regulatory influence dropped from 17% to 12%.

---

## 3.4 RAI in Academia

The number of responsible AI papers accepted at six leading AI conferences grew 19%, from 1,278 to 1,521, between 2024 and 2025. Security and safety has become the largest and fastest growing area of RAI research, with 641 accepted papers, a 23% increase from 2024. Fairness and bias accounted for 462 (+13%), transparency and explainability for 405 (+14%), and privacy and data governance for 248 (+33%).

In 2025, China led with 812 accepted RAI papers, more than double the 394 from the United States. In 2024, the United States had led with 788 papers to China's 322. The reversal is sharp, but consistent with China's lead in overall AI publication volume.

---

## 3.5 RAI Policymaking

Responsible AI governance depends on countries both adopting ethical principles and having the institutions and regulations to enforce them. UNESCO's Readiness Assessment Methodology (RAM) evaluates national readiness across dimensions such as legal frameworks, technical infrastructure and education.

Most major AI-producing countries, including the United States, China, and much of Western Europe, have not participated in the assessment. Countries that have are concentrated in Latin America, Sub-Saharan Africa, and parts of South and Southeast Asia.

### Highlight: Global AI Governance Participation

Since 2019, international cooperation on AI governance has become more widespread, but the depth of engagement varies significantly. Only five countries—Canada, France, Germany, Italy, and Japan—have consistently endorsed every major global AI governance initiative recorded between 2019 and 2025.

The 2025 AI Action Summit in France marked a further turning point, convening over 100 countries alongside civil society organizations and NGOs. Sixty-four participants signed the resulting Statement on Inclusive and Sustainable AI, including the African Union Commission and the European Union. In a notable shift, both the United States and the United Kingdom declined to sign the final declaration. The UK cited a lack of emphasis on national security, while the U.S. decision reflected a pivot toward a more deregulatory, "innovation-first" approach.

---

## 3.6 Data Governance for Privacy

The Global Index on Responsible AI (GIRAI) covers 138 countries and scores them on a 0 to 100 scale across thematic areas. Countries fall across a wide spectrum, with GIRAI data protection and privacy scores ranging from near zero to above 80. Australia and parts of Europe score the highest, while parts of Africa and the Middle East show an absence of dedicated data protection legislation. Most countries now have some form of data protection legislation in place, though a few, mostly in Africa and parts of Asia, are still in draft stages or have no legislation at all.

---

## 3.7 Fairness and Bias

### Bias and Unfair Discrimination

GIRAI scores on the bias and unfair discrimination dimension are fairly low across the board. The United States and Canada score highest, with Australia, parts of Europe, and Brazil falling in the middle range. Much of Africa, the Middle East, and Central Asia score below 20.

### Gender Equality

Canada and The Netherlands score the highest on GIRAI's gender equality dimension. Parts of Europe and Japan fall in the 61–80 range, followed by countries like the United States and Brazil, which score from 41–60.

### Cultural and Linguistic Diversity

Singapore scores the highest on GIRAI's cultural and linguistic diversity dimension, while Germany, Ireland, Italy, Qatar, Estonia, and Slovenia also score in the upper ranges (70–88). In Africa, nonstate actors show activity in 39% of countries, but only 7% have government programs and just 2% have legal frameworks in place.

### Highlight: Inclusiveness and the Global Language Gap

As a small number of proprietary models shape global AI capabilities, the "global language gap" has become more visible. These systems perform much better in English and a handful of other widely spoken languages than in all others.

On HELM Arabic, the top-scoring model was Arabic.ai's LLM-X, a regionally developed model, with a mean score of 0.86, ahead of Gemini 2.5 Flash (0.82) and GPT-5.1 (0.81). Rankings that hold in English-centric evaluations do not necessarily hold when benchmarks reflect local usage, dialect, and cultural references.

The gap extends beyond language boundaries to dialect variation within the same language. On the Slovene DIALECT-COPA benchmark, GPT-5 scored 99.8% on Standard Slovenian but dropped to 88.6% on the Cerkno dialect. The drop was steeper for other models: Mistral Medium 3.1 fell from 90.0% to 53.2%, and Llama 3.3 fell from 87.0% to 53.6%.

**Selected regional AI benchmarks:**

**Africa:**
- AfroBench — 64 African languages — Multi-task LLM evaluation across NLU, generation, QA/knowledge, and math
- IrokoBench — 17 low-resource African languages — Human-translated suite covering NLI, math reasoning, and multi-choice knowledge QA

**Asia, MENA, Central Asia:**
- Indic LLM Arena — Many Indian languages — Crowd-sourced evaluation of language, culture, and safety in Indian contexts
- HELM Arabic — Arabic — Transparent, reproducible Arabic LLM evaluation leaderboard
- SEA-HELM — Filipino, Indonesian, Tamil, Thai, Vietnamese — Southeast Asian holistic evaluation across multiple tasks
- TUMLU — Azerbaijani, Crimean Tatar, Karakalpak, Kazakh, Kyrgyz, Tatar, Turkish, Uyghur, Uzbek — Natively developed multilingual benchmark for Turkic languages

**Europe:**
- BenCzechMark — Czech — Comprehensive Czech-centric benchmark with 50 tasks
- IberBench — Basque, Catalan, Galician, Spanish, Portuguese, English — Large, extensible benchmark integrating 101 datasets
- SloBENCH — Slovenian — Evaluation platform including DIALECT-COPA (standard vs. dialect)

---

## 3.8 Transparency

### The Openness Index

The Artificial Analysis Openness Index scores AI models on a 0 to 100 scale based on how freely weights can be accessed and licensed, as well as transparency around training methodology and pre- and post-training data. Scores are low across leading models, with most falling between 2 and 16 out of 100. K2 Think and Olmo 3 32B Think scored the highest, and they are also the only two models that scored any points for pre-training data transparency.

### Foundation Model Transparency Index

In the 2025 edition, average transparency declined from 58 in 2024 to 40. IBM leads at 95 and Writer follows at 72. Others, such as xAI and Midjourney, score just 14. The weakest area is Upstream, particularly around training data and the resources used to build models.

**Foundation Model Transparency Index scores, 2025 (selected):**

| Organization | Model           | Score |
| ------------ | --------------- | ----: |
| IBM          | Granite 3.3     |    95 |
| Writer       | Palmyra X5      |    72 |
| AI21 Labs    | Jamba 1.6       |    66 |
| Anthropic    | Claude 4        |    46 |
| Google       | Gemini 2.5      |    41 |
| Amazon       | Nova Premier    |    39 |
| OpenAI       | o3              |    35 |
| DeepSeek     | DeepSeek-R1     |    32 |
| Meta         | Llama 4         |    31 |
| Alibaba      | Qwen 3          |    26 |
| Mistral      | Medium 3        |    18 |
| Midjourney   | Midjourney V7   |    14 |
| xAI          | Grok 3          |    14 |

---

## 3.9 Security and Safety

### Global AI Safety Institutes

Fully operational AI safety institutes (AISIs) now exist in the UK (AI Security Institute), the U.S. (USAISI at NIST), Japan (JAISI), Singapore (Digital Trust Centre), and Israel (AI Security Research Unit). India and France have also launched AISIs. A second wave is in development in Canada, South Korea, Germany, and Brazil.

### Benchmarks

#### HELM Safety

Most models released between 2024 and 2025 score between 0.90 and 0.98 on HELM Safety, with a very narrow gap between the highest and lowest scorers. The overall trajectory suggests that leading models are converging on a safety ceiling where current benchmarks may not be fine-grained enough to distinguish meaningful differences.

#### AILuminate

Among models tested with external guardrails in place, Claude 3.5 Haiku, Claude 3.5 Sonnet, and Mistral Large all received "Very Good" ratings, while several others received "Good." Among models tested without external safety filters, Gemma 2 9b, Phi 3.5 MoE Instruct, and Phi 4 scored "Very Good."

**AILuminate safety grades (with external guardrails):**

| Model                          | Grade     |
| ------------------------------ | --------- |
| Claude 3.5 Haiku 20241022      | Very Good |
| Claude 3.5 Sonnet 20241022     | Very Good |
| Mistralai Mistral Large 2402 Moderated | Very Good |
| Amazon Nova Lite v1.0          | Good      |
| Gemini 1.5 Pro (API)           | Good      |
| GPT-4o                         | Good      |
| GPT-4o mini                    | Good      |

#### Jailbreak T2T Benchmark v0.5

Under normal conditions, most models score in the "very good" or "good" range. After jailbreak attempts, nearly every system's score drops, some by a full tier or more. So while safety under normal use is generally good, it degrades under deliberate manipulation.

---

## 3.10 Tradeoffs Across RAI Dimensions

In practice, AI systems must satisfy multiple responsible AI dimensions at once. A growing number of empirical research studies suggest that these dimensions do not improve independently, as optimizing for one can degrade others.

Kemmerzell and Schreiner (2024) tested this directly by training image classification models on four facial analysis data sets. Differential privacy improved privacy scores across all datasets but reduced explainability, fairness, and accuracy, with accuracy falling by up to 33 percentage points on some configurations. Training adaptations aimed at improving fairness only succeeded on the dataset with the most demographic imbalance.

A separate evaluation of large language models found a similar pattern. GPT-4 led on robustness (0.91) and accuracy (0.67), but Llama 2 7B scored highest on toxicity avoidance (0.98). Models that performed well on robustness, such as Mistral 7B and Mixtral 8×7B, scored among the lowest on toxicity avoidance (0.39 and 0.42).

These trade-offs also appear in federated learning. In an Alzheimer's disease scenario, adding stronger privacy protections reduced the model's ability to correctly identify the disease, with accuracy falling by 14.8 percentage points. The effect was worse for hospitals with less data, where missed diagnoses rose by 21.4%.

There was not a single intervention method that proved to improve all four dimensions at once. There is no shared framework that measures or compares these trade-offs, which is another measurement gap in the RAI space.

---

# Chapter 4: Economy

## Overview

In 2025, more money flowed into AI than ever before, and faster. Global corporate AI investment more than doubled, revenue at leading frontier companies grew at historically fast rates. Generative AI reached close to 53% population-level adoption within three years of its mass-market introduction, faster than the personal computer or the internet, and that rapid uptake is translating into real value. U.S. consumer surplus from generative AI reached an estimated $172 billion annually by early 2026. But the benefits of this expansion are not distributed evenly. Investment is heavily concentrated in a small number of countries, companies and deals. In labor markets, demand for AI skills is rising across sectors but the workforce impact is showing signs of falling disproportionately on the youngest workers in AI-exposed occupations. Productivity gains are measurable within narrow tasks, but the evidence at the macro level remains early and mixed.

## Chapter Highlights

1. **Global corporate AI investment more than doubled in 2025.** Private investment grew fastest at 127.5% and now accounts for 60% of the total. Generative AI led the surge, growing more than 200% and capturing nearly half of all private AI funding. Newly funded AI companies rose 71%, and billion-dollar funding events nearly doubled.

2. **The United States continues to lead in global private AI investment, committing 23 times more than China.** In generative AI, U.S. investment exceeded the combined total of China and Europe by a wide margin. However, private investment figures likely understate China's total AI spending, as government guidance funds have deployed an estimated $184 billion into AI firms between 2000 and 2023.

3. **AI company revenue is rising at historically fast rates, but compute costs and infrastructure spending are also reaching record levels.** Major cloud providers have accelerated capital expenditures, with Google reporting more than $150 billion in annual capex in 2025.

4. **The value consumers get from generative AI grew 54% in a year.** Estimated U.S. consumer surplus reached $172 billion annually by early 2026, up from $112 billion a year earlier, with the median value per user tripling over the same period.

5. **Organizational AI adoption continued to rise in 2025, up to 88% of surveyed organizations, though AI agent use remains early.** Generative AI is now used in at least one business function at 70% of organizations. AI agent deployment was in the single digits across nearly all business functions.

6. **Generative AI reached 53% adoption in three years, faster than the personal computer or the internet.** Adoption varies widely across countries and correlates strongly with GDP per capita, though some outpace what income would predict, including Singapore at 61% and the United Arab Emirates at 54%. Despite its lead in AI investment and model development, the United States ranks 24th at 28.3%.

7. **AI's labor market effects are showing up unevenly, concentrated in hiring pipelines and the youngest workers in exposed occupations.** Employment for software developers ages 22 to 25 has fallen nearly 20% from 2024. Employer surveys point to further change ahead, with one-third of respondents expecting workforce reductions over the coming year.

8. **One-third of organizations expect AI to reduce their workforce in the coming year, even though large-scale job losses have not yet shown up in overall employment data.** Almost half of organizations surveyed expected little to no change. Anticipated reductions are highest in service operations, supply chain, and software engineering.

9. **Productivity gains from AI are largest in structured, measurable work where outputs are easy to monitor.** Studies report gains of 14% to 15% in customer support, 26% in software development, and 50% in marketing output. Gains are smaller in tasks requiring deeper reasoning, and recent evidence raises concerns that heavy AI reliance may carry long-term learning penalties that slow skill development over time.

10. **China continues to install more industrial robots than the rest of the world combined, and the gap widened in 2024.** China accounted for 54% of industrial robots installed globally, up from 51.1% in 2023. Taiwan recorded the highest year-over-year growth at 33%.

---

## 4.1 Year in Review: 2025

**Key milestones and deals:**

| Date | Event |
| ---- | ----- |
| Jan 21 | **$500B Stargate Project** — OpenAI, SoftBank, Oracle, and MGX launch Stargate, planning to invest $100B–$500B in advanced AI data centers across the U.S. by 2029 |
| Jan 27 | **DeepSeek** reaches No. 1 on Apple's U.S. App Store |
| Mar 6 | **$138B** — China announces a $138B state VC fund to invest in AI and cutting-edge technologies |
| Mar 28 | **$23B** — CoreWeave has the largest U.S. tech IPO since 2021, raising $1.5B at a $23B valuation |
| Mar 31 | **$300B** — OpenAI raises $40B at a $300B post-money valuation |
| May 13 | **$5B** — AWS and HUMAIN announce a $5B AI infrastructure deal to accelerate AI adoption in Saudi Arabia and globally |
| May 21 | **$6.5B** — OpenAI acquires IO, the AI hardware startup founded by Jony Ive, for $6.5B |
| Jun 2 | **Watsonx AI** — IBM acquires the AI startup Seek AI to launch Watsonx AI Labs |
| Jul 9 | **$4T** — Nvidia becomes the first public company worth $4 trillion |
| Jul 15 | **$12B** — Thinking Machines Lab, founded by Mira Murati, raises a $2B seed round at a $12B valuation |
| Sep 2 | **$183B** — Anthropic raises $13B in Series F at a $183B post-money valuation |
| Sep 10 | **$300B** — OpenAI signs a $300B, five-year cloud contract with Oracle |
| Oct 27 | **$10B** — Mercor raises $350M Series C at a $10B valuation; its founders, both 22 years old, become the youngest ever self-made billionaires |
| Nov 14 | **$29.3B** — Anysphere (Cursor) raises $2.3B at a $29.3B valuation |
| Nov 14 | **$40B** — Google announces a $40B investment in Texas data centers and AI infrastructure through 2027 |
| Nov 20 | **$5.6B** — Physical Intelligence raises $600M at a $5.6B valuation |
| Dec 9 | **$17.5B** — Microsoft pledges a $17.5B investment in India's AI infrastructure |
| Dec 10 | **$35B** — Amazon announces it will invest over $35B in India by 2030 |
| Dec 22 | **$4.75B** — Alphabet says it will acquire Intersect for $4.75B to accelerate U.S. energy innovation and data center infrastructure |

---

## 4.2 Investment and Infrastructure

### Corporate Investment Activity

Global corporate AI investment increased approximately fortyfold since 2013. In 2025, total investment reached $581.69 billion, marking a 129.9% increase from the previous year. Private investments represented the largest share with $344.66 billion, up 127.5% from 2024. Mergers and acquisitions showed similar signs of growth, rising 132.6% year over year.

### Private Investment Activity

In 2025, global private investment in AI reached $344.7 billion, a 127.5% increase over the previous year. Generative AI companies accounted for $170.9 billion of that total, representing nearly half of all private investment and an increase of over 200% from 2024.

---

The private investment market is expanding in breadth but even more so in concentration. While the absolute number of newly funded AI companies has grown in 2025 (70.8% year-over-year increase), distribution of capital has dropped and the majority of investment dollars flow through a small number of deals. Compared to 2024, the average private AI investment event in 2025 increased 46% to $66.5 million. Investment activity increased across all funding sizes, but the strongest growth was at the upper end of that distribution, with 28 events exceeding $1 billion, up from 15 in 2024.

**Number of newly funded AI companies in the world, 2013–25** *(Fig. 4.2.4)*

| Year | Companies |
| ---- | --------- |
| 2013 | ~480      |
| 2025 | 3,499     |

**Number of newly funded generative AI companies, 2019–25** *(Fig. 4.2.5)*

| Year | GenAI companies |
| ---- | --------------- |
| 2019 | ~25             |
| 2024 | ~365            |
| 2025 | 311             |

**Average size of global AI private investment events, 2013–25** *(Fig. 4.2.6)*

Average investment per event rose from ~$12M in 2013 to **$66.52M in 2025**.

**Global AI private investment events by funding size, 2024 vs. 2025** *(Fig. 4.2.7)*

| Funding size (USD)      | 2024  | 2025  |
| ----------------------- | ----- | ----- |
| Over $1 billion         |    15 |    28 |
| $500M – $1 billion      |    20 |    30 |
| $100M – $500M           |   146 |   286 |
| $50M – $100M            |   197 |   373 |
| Under $50M              | 2,951 | 4,464 |
| Undisclosed             |   209 |   324 |
| **Total**               | **3,538** | **5,505** |

### Geographic Distribution of Private Investment

As measured by both investment totals and the number of newly funded companies, private AI investment remains highly concentrated in a small number of countries. In 2025, the United States was the global leader with nearly $285.9 billion total invested, 23.1 times greater than the next highest country, China ($12.4 billion), and 48.5 times the amount invested in the United Kingdom ($5.9 billion). The United States led with 1,953 newly funded AI companies in 2025, compared to 172 in the United Kingdom and 161 in China. More than half of total private AI investment in the United States was generative AI-related ($163.6 billion), while the combined investment by China and Europe was $4.7 billion. Since 2024, private AI investment in the United States increased 160.2%, compared to an increase of 32.2% in China and 7.2% in Europe.

**Global private investment in AI by geographic area, 2025** *(Fig. 4.2.8)*

| Country      | Investment ($B) |
| ------------ | --------------- |
| 🇺🇸 United States | 285.88 |
| 🇨🇳 China    |   12.41 |
| 🇬🇧 United Kingdom |  5.90 |
| 🇫🇷 France   |    4.36 |
| 🇨🇦 Canada   |    4.28 |
| 🇮🇳 India    |    4.09 |
| 🇩🇪 Germany  |    3.89 |
| 🇮🇱 Israel   |    3.58 |
| 🇦🇺 Australia |   2.52 |
| 🇸🇦 Saudi Arabia |  2.03 |
| 🇸🇬 Singapore |   1.82 |
| 🇰🇷 South Korea |  1.78 |
| 🇧🇪 Belgium  |    1.20 |
| 🇯🇵 Japan    |    1.11 |
| 🇸🇪 Sweden   |    0.97 |

**Number of newly funded AI companies by geographic area, 2025** *(Fig. 4.2.9)*

| Country        | Companies |
| -------------- | --------- |
| United States  |     1,953 |
| United Kingdom |       172 |
| China          |       161 |
| India          |       108 |
| Germany        |        92 |
| France         |        84 |
| Canada         |        79 |
| Israel         |        64 |
| South Korea    |        59 |
| Japan          |        56 |
| Singapore      |        49 |
| Italy          |        38 |
| Australia      |        38 |
| Switzerland    |        34 |
| Spain          |        33 |

Since 2013, the United States has attracted $757.3 billion in total private AI investment, far ahead of China at $131.8 billion. Other countries with notable cumulative totals include the United Kingdom ($34.1B), Canada ($19.6B), Israel ($18.5B), and Germany ($17.2B). The private investment figures in this section are drawn from Quid and do not account for government-backed funding in countries like China. Between 2000 and 2023, it was estimated that $912 billion in government guidance funds were deployed across industries, with an estimated $184 billion allocated towards AI companies.

**Global private investment in AI by geographic area, 2013–25 (sum)** *(Fig. 4.2.12)*

| Country              | Total ($B) |
| -------------------- | ---------- |
| United States        |     757.27 |
| China                |     131.83 |
| United Kingdom       |      34.07 |
| Canada               |      19.59 |
| Israel               |      18.54 |
| Germany              |      17.16 |
| France               |      15.57 |
| India                |      15.39 |
| South Korea          |      10.75 |
| Singapore            |       9.09 |
| Sweden               |       8.24 |
| Japan                |       7.00 |
| Australia            |       6.50 |
| Switzerland          |       4.73 |
| United Arab Emirates |       4.24 |

Within the United States, California accounted for $218 billion in 2025 (over 75% of national total). Colorado ($19B), New York ($13B), and Florida ($6B) followed. More than half of all U.S. states received less than $100 million in private AI investment; South Dakota, Oklahoma, Arkansas, and West Virginia reported no mapped investment activity.

### Focus Areas of Private Investment

In 2025, the breakdown of private AI startup investment by focus area shows capital directed most heavily toward AI infrastructure, models, research, and governance, reaching **$143.2 billion** — the largest share of any category. Other notable areas include data management and processing ($31.58B), IoT ($14.63B), medical and healthcare ($11.75B), and pharmaceutical ($10.58B). The infrastructure category has experienced the steepest growth compared with all other areas.

**Global private investment in AI by focus area (2025 selected values)** *(Fig. 4.2.17)*

| Focus area                                  | 2025 ($B) |
| ------------------------------------------- | --------- |
| AI infrastructure/models/research/governance |   143.22 |
| Data management, processing                 |     31.58 |
| Internet of Things                          |     14.63 |
| Medical and healthcare                      |     11.75 |
| Cloud computing                             |     10.31 |
| Pharmaceutical                              |     10.58 |
| Cybersecurity, data protection              |      8.42 |
| Autonomous vehicles                         |      7.94 |
| Robotics                                    |      7.84 |
| Fintech                                     |      6.52 |
| AI agents                                   |      8.02 |
| Defense                                     |      5.30 |
| Semiconductors                              |      4.40 |
| Energy management                           |      4.64 |
| Creative, music, video content              |      4.19 |

### AI Company Economics

#### Revenue

Annualized revenue estimates for leading AI companies have grown quickly in recent years. As of early 2026: OpenAI at **$25B**, Anthropic at **$19B**, xAI at **$428M**, Mistral AI at **$400M**, and Z.ai at **$53M**. OpenAI's early revenue growth outpaces that of Uber, Cheniere Energy, and Moderna over comparable time periods after crossing $1B in annual revenue. Google remains the only company in the comparison set scaling toward $100B in annual revenue.

#### Annual Compute Spend

Reported annual compute spend increased significantly from 2024 to 2025 for frontier AI companies.

**Annual compute spend of select frontier AI companies** *(Fig. 4.2.20)*

| Company    | Year | R&D ($B) | Inference ($B) | Unattributed ($B) | Total ($B) |
| ---------- | ---- | --------- | -------------- | ----------------- | ---------- |
| OpenAI     | 2022 |           |                |                   |       0.42 |
| Anthropic  | 2022 |           |                |                   |       0.28 |
| OpenAI     | 2024 |      4.00 |           1.80 |                   |       5.80 |
| Anthropic  | 2024 |      1.50 |                |              2.50 |       4.00 |
| OpenAI     | 2025 |      8.30 |           8.00 |                   |      16.30 |
| Anthropic  | 2025 |      4.10 |           2.70 |                   |       6.80 |

#### Capital Expenditures

The infrastructure needed to support frontier AI is financed not only by AI companies but also by the major cloud providers. In 2025, Google and Amazon led in total annual capital expenditures (capex), with Google reporting more than $150 billion in capex. Hyperscalers' annual capex has more than doubled since ChatGPT's release, with combined capex for AMZN, META, GOOGL, MSFT, and ORCL projected to exceed $400B by 2026.

> **HIGHLIGHT: What Is Generative AI Worth?**
>
> Investment, revenue, and compute costs measure the value of AI to companies building and deploying it, but not what it's worth to users. Brynjolfsson et al. (2026) provide the first longitudinal estimates of that value using online choice experiments. The study finds that total consumer surplus from generative AI is estimated to have grown from **$112 billion to $172 billion** annually in the United States. U.S. adults using generative AI grew from 95M to 115M (+21%). The average consumer surplus per user increased 27% from $98 (2025) to $125 (2026), and the median value per user tripled from $3.40 to $11.40. Usage frequency is the strongest individual predictor of surplus, followed by work use and number of products used.

---

## 4.3 Corporate AI Adoption

The investment and infrastructure activity described earlier establishes the scale of resources being directed toward AI. This section examines how those investments translate into organizational use and reported business outcomes, drawing on McKinsey & Company's annual State of AI surveys.

### Industry Usage

In 2025, organizational adoption of AI continued to expand in both usage and function. A large majority of respondents reported that their organization uses AI in at least one business function, up to **88%** in 2025 from 78% in 2024. Over half of respondents reported three or more business functions leveraging AI. Use of generative AI mirrored that growth, with 79% of respondents reporting that their organizations regularly use generative AI in at least one business function, compared to 71% in 2024. China and Europe experienced higher year-over-year increases (13 and 11 percentage points, respectively).

**AI use by organizations by region, 2025** *(Fig. 4.3.2)*

| Region                             | 2025 | 2024 | 2023 |
| ---------------------------------- | ---- | ---- | ---- |
| All geographies                    |  88% |  73% |  55% |
| Asia-Pacific                       |  82% |  72% |  58% |
| Europe                             |  91% |  80% |  57% |
| North America                      |  90% |  62% |  61% |
| Greater China (incl. HK, TW, MO)   |  88% |  75% |  48% |
| Developing markets (incl. India, Central/South America, MENA) | 88% | 77% | 49% |

### By Industry and Function

The highest reported AI usage was in knowledge management for business, legal, and professional services (58%) and in software engineering and IT in the technology sector (58% and 56%, respectively). Marketing and sales for consumer goods and retail was closely behind (51%). Functions tied to information processing, software, and customer engagement reported higher adoption than strategy, corporate finance, and compliance, where uptake remains low across most sectors. Financial services were an exception, with high use in risk and compliance functions.

**AI use by industry and function, 2025** *(Fig. 4.3.3)*

| Industry                                | HR  | IT  | Knowledge Mgmt | Manufacturing | Marketing & Sales | Product Dev | Risk/Legal | Service Ops | Software Eng | Strategy | Supply Chain |
| --------------------------------------- | --- | --- | -------------- | ------------- | ----------------- | ----------- | ---------- | ----------- | ------------ | -------- | ------------ |
| Advanced industries                     | 18% | 40% | 29%            | 26%           | 29%               | 30%         | 7%         | 22%         | 32%          | 16%      | 25%          |
| Business, legal, and professional svcs  | 20% | 21% | **58%**        | 1%            | 46%               | 33%         | 15%        | 32%         | 13%          | 22%      | 4%           |
| Consumer goods and retail               | 22% | 32% | 28%            | 13%           | **51%**           | 21%         | 11%        | 34%         | 19%          | 9%       | 22%          |
| Energy and materials                    | 22% | 39% | 33%            | 21%           | 33%               | 28%         | 17%        | 32%         | 30%          | 20%      | 19%          |
| Financial services                      | 17% | 36% | 38%            | 0%            | 36%               | 28%         | **43%**    | 38%         | 23%          | 12%      | 3%           |
| Health care, pharma, and medical        | 25% | 30% | 46%            | 11%           | 38%               | 36%         | 13%        | 25%         | 21%          | 18%      | 21%          |
| Media and telecom                       | 28% | 38% | 34%            | 5%            | 45%               | 32%         | 17%        | 46%         | 33%          | 17%      | 6%           |
| Technology                              | 28% | 56% | 46%            | 9%            | 49%               | 49%         | 18%        | 45%         | **58%**      | 20%      | 10%          |

Respondents more often associated AI with the highest cost savings in software engineering and manufacturing functions (56%), while revenue gains were cited with marketing and sales (67%), strategy and corporate finance (65%), and product/service development (62%). Across broader organizational outcomes, 64% reported that AI usage had improved innovation, and 45% reported improvements in employee and customer satisfaction. Negative effects were reported less frequently, with no more than 7% believing AI usage had worsened cost metrics.

### Deployment Stages

The McKinsey survey also captured how deeply AI had been integrated by looking at different stages of the deployment life cycle. Larger companies were the most likely to report that their AI programs had reached a scaling phase. Early indicators on AI agent adoption show that diffusion is still at an early stage — across most business functions, a majority of respondents reported no agent use at all. The technology sector had comparatively higher rates of scaled agent use in software engineering (24%), IT (22%), and service operations (21%).

> **HIGHLIGHT: Measuring Signals of AI Diffusion**
>
> Compared with earlier transformative technologies, generative AI's adoption has been rapid. Measured from the release of the first widely available product, generative AI reached approximately **53% adoption within three years**, well above the initial trajectories of the personal computer and the internet over comparable time frames. AI diffusion also varies widely across countries and shows a strong, statistically significant positive correlation with GDP per capita. Most high-income economies cluster between 25% and 45% adoption. The United Arab Emirates (64%) and Singapore (61%) report adoption levels well above what their GDP per capita would predict. The United States, despite leading in AI investment, dropped to 24th place with a population-level adoption rate of 28.3% in the second half of 2025.
>
> **AI diffusion rankings, second half 2025 (top 10):** UAE (64.0%), Singapore (60.9%), Norway (46.4%), Ireland (44.6%), France (44.0%), Spain (41.8%), New Zealand (40.5%), Netherlands (38.9%), UK (38.9%), Qatar (38.3%).
>
> Platform-level data from Anthropic's AI Usage Index shows that computer and mathematical tasks accounted for close to 40% of overall usage in 2025. Educational instruction and library tasks showed the most significant growth, rising from 9% early in the year to ~14% by late 2025. The share of automation-oriented conversations rose from 41% (Jan) to 49% (Aug), then fell back to 45% (Nov), with augmentation-style interactions at 52% in November.

---

## 4.4 Jobs

Labor markets provide signals of how investment, technical progress, and organizational adoption are changing workforce dynamics. This section tracks both the demand side (job postings and skill requirements) and the supply side (talent flows and employment outcomes), drawing from Lightcast's job posting database and LinkedIn's talent and hiring metrics.

### AI Labor Demand Across Geographies

Demand for AI-related talent continued to increase in 2025 as job listings requiring AI skills make up a growing share of overall postings. In 2025, **Singapore led with 4.69%** of all job postings requiring AI skills, followed by Hong Kong (3.5%), Luxembourg (3.4%), and Spain (3.3%). The United States reached 2.6%, followed by Chile (2.4%) and the United Kingdom (1.9%).

### AI Hiring in the United States

#### Skill Composition

Within the United States, broad AI and machine learning skill clusters remain the most frequently cited categories in AI job postings, accounting for 1.7% and 1.0% of all job postings. Among top specialized skills, **Python** appeared most often in 258,674 posts — a 391% increase compared to 2013–15. The fastest growth appears in skills needed to build and operate systems at scale: Amazon Web Services (+1,358%), scalability (+733%), and workflow management (+818%).

Mentions of generative AI skills in AI job postings grew 111% from 2024 to 2025, though their share of total AI job postings decreased by 5%. A new skill cluster tied to AI agents emerged: postings referencing agentic AI (+10,854%), AI agents (+1,062%), or LangGraph (+2,113%) grew exponentially. Job demand is shifting from general familiarity with chat-based tools toward skills required to coordinate and operationalize task-oriented systems.

**Top 10 specialized skills in 2025 AI job postings (US), 2013–15 vs. 2025** *(Fig. 4.4.4)*

| Skill                  | 2013–15 | 2025     | Growth  |
| ---------------------- | ------- | -------- | ------- |
| Python                 |  52,878 |  258,674 |  +391%  |
| Computer science       |  97,085 |  257,127 |  +165%  |
| Scalability            |  23,727 |  197,744 |  +733%  |
| Automation             |  25,870 |  190,758 |  +610%  |
| Workflow management    |  20,295 |  186,325 |  +818%  |
| Data analysis          |  54,942 |  170,396 |  +210%  |
| SQL                    |  65,300 |  151,191 |  +132%  |
| Project management     |  60,745 |  149,865 |  +147%  |
| Data science           |  26,767 |  142,120 |  +431%  |
| Amazon Web Services    |   9,742 |  142,037 | +1,358% |

**Generative AI skills in AI job postings (US), 2024 vs. 2025** *(Fig. 4.4.5)*

| Skill                        | 2024   | 2025    | Growth  |
| ---------------------------- | ------ | ------- | ------- |
| Generative artificial intelligence | 65,557 | 138,188 | +111% |
| Large language modeling      | 19,045 |  38,526 |  +102%  |
| Prompt engineering           |  6,152 |  22,227 |  +261%  |
| Retrieval augmented generation | 2,895 |  12,609 |  +337%  |
| Context engineering          |      9 |     703 | +7,711% |

**AI agent skills in AI job postings (US), 2024 vs. 2025** *(Fig. 4.4.7)*

| Skill               | 2024  | 2025   | Growth    |
| ------------------- | ----- | ------ | --------- |
| Agentic AI          |   151 | 16,541 | +10,854%  |
| AI agents           | 1,310 | 15,217 |  +1,062%  |
| ChatGPT             | 5,935 | 14,376 |    +160%  |
| Conversational AI   | 5,430 |  6,976 |     +28%  |
| Microsoft Copilot   | 1,416 |  6,395 |    +352%  |
| Multi-agent systems | 1,635 |  5,461 |    +234%  |
| LangGraph           |   194 |  4,294 |  +2,113%  |
| Agentic systems     |   192 |  2,850 |  +1,384%  |

#### By Sector

Demand for AI talent increased across all economic sectors in 2025. The **information sector leads**, with AI skills appearing in 13.2% of job postings, up from 7.8% in 2024. Other high sectors include professional, scientific, and technical services (6.5%), finance and insurance (5.3%), and manufacturing (4.7%).

#### By State

California leads with 170,881 AI job postings (17.2% of national total). Texas follows with 80,547 postings (8.1%) and New York with 66,029 (6.6%). These three states represent approximately a third of all AI job postings nationally. Washington D.C. accounts for a comparatively high 6.2% share of AI postings within its local labor market, followed by Delaware at 4.4%.

### AI Hiring

In most countries, AI hiring rates outpaced overall hiring growth in 2025. Indonesia recorded the highest relative AI hiring growth at 31.7%, followed by Croatia (27.8%) and Belgium (21.5%). Since 2018, many countries show a sustained pattern of AI hiring rates exceeding general labor market growth; Iceland and Sweden are exceptions where AI hiring growth lagged.

### AI Talent Concentration

In 2025, **Israel had the highest concentration of AI talent** among LinkedIn members (2.1%), followed by Singapore (1.8%) and Luxembourg (1.6%). The United Arab Emirates, India, and Saudi Arabia showed the fastest growth in their share of AI talent, each increasing over 100% between 2019 and 2025. Luxembourg recorded the highest net inflow of AI talent at 5.23 per 10,000 LinkedIn members. The United States is a net importer of AI talent at 1.2 per 10,000 LinkedIn members.

Gender representation within AI talent continues to be uneven. Men still account for the majority of AI talent, typically between 65% and 75%. Gender ratios have for the most part stayed flat since 2016, despite an expanding AI workforce. In the United States, women represent a 34.3% share of AI talent compared to men's 65.7%.

### AI's Labor Impact

#### Productivity Trends

A growing body of academic research has begun to measure AI's impact both at the micro level (how individual workers perform their jobs using AI tools) and at the macro level (aggregate productivity and employment). Results have generally been positive for structured, language-heavy tasks:

- Customer support agents using conversational AI resolved 14–15% more issues per hour
- Software developers using GitHub Copilot completed 26% more pull requests
- Marketing teams using multimodal AI for ad creation saw a 50% increase in output per worker
- One consistent finding: less experienced workers tended to benefit the most, suggesting AI tools may help close existing skill gaps

Results are not uniformly positive. Model Evaluation & Threat Research (METR) found that experienced open-source developers became 19% slower when using AI assistance — though a later study found developers were increasingly reluctant to work without AI, and those in late 2025 were likely already sped up by AI relative to the original study period. AI's productivity effects are highly context dependent; gains are strongest when work can be divided into well-defined, repeatable tasks with clear quality monitoring.

---

**Selected micro-level productivity studies** *(Fig. 4.4.27)*

| Study                    | Occupation          | AI application           | Change in productivity                              | Who benefited most?                                    |
| ------------------------ | ------------------- | ------------------------ | --------------------------------------------------- | ------------------------------------------------------ |
| Reimers & Waldfogel 2026 | Authors             | LLMs for content         | +200% (output volume; releases tripled)             | New entrants (drove quantity); pre-AI (maintained quality) |
| Shen & Tamkin 2025       | Software engineers  | Learning new libraries   | 0% (statistically insignificant)                   | High scorers (65%+) who used AI for conceptual inquiry; "learning penalties" |
| Becker et al. 2025       | Developers          | Open-source tools        | -19% (speed; developers became slower with AI)     | None (gap between perceived help and actual performance) |
| Brynjolfsson et al. 2025 | Support agents      | Conversational assistant | +14%–15% (issues resolved per hour)                | Less experienced/skilled agents (30%–35% gains)        |
| Cui et al. 2025          | Software developers | GitHub Copilot           | +26% (completed pull requests)                     | Junior and less-experienced workers                     |
| Ju & Aral 2025           | Marketing teams     | Multimodal ad creation   | +50% (productivity per worker)                     | Human-AI teams (shifted focus from social coordination) |
| Choi & Xie 2025          | Accountants         | AI-based accounting      | +55% (weekly client support)                       | Experienced accountants (used AI confidence scores)    |

#### Macro-level Studies

At the macro level, the evidence is earlier and less conclusive, but there are indicators that AI is starting to register in aggregate productivity data. A study of 12,000 European firms found that AI adoption boosted labor productivity by 4%, with training strengthening the outcome. In the United States, productivity growth reached **2.7%** in 2025, nearly double the 1.4% average of the previous decade. OECD projections for G7 economies estimate annual productivity gains of 0.2 to 1.3 percentage points over the next decade. A survey of 6,000 executives across four countries found widespread adoption but minimal realized productivity gains and a projected 0.7% reduction in employment over the next three years.

**Selected macro-level studies** *(Fig. 4.4.28)*

| Study                        | Scope                        | Insight                                                                               | Impact                                                 |
| ---------------------------- | ---------------------------- | ------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| Aldasaro et al. 2026         | 12,000 European firms        | AI adoption increases efficiency without reducing short-run employment; training boosts | +4% labor productivity; +5.9pp gain for training spend |
| Yotzov et al. 2026           | 6,000 executives (US, UK, DE, AU) | High adoption but minimal realized productivity impact to date                   | +1.4% productivity boost; +0.8% output; -0.7% employment (3-yr projection) |
| Brynjolfsson 2026            | United States economy        | "J-curve" — absorbing adoption costs before gains appear                              | 2.7% US productivity growth 2025                       |
| Filippucci et al. OECD 2025  | G7 economies (10-yr horizon) | Projected gains based on sectoral specialization                                      | +0.4 to +1.3 pp annual labor productivity growth       |
| Frank et al. 2026            | LinkedIn profiles + US UI records | Deterioration in AI-exposed labor markets began early 2022                        | Negative entry rates for AI-exposed roles               |
| Brynjolfsson et al. 2025     | ADP payroll data through 2025 | "Canaries in the coal mine": large employment declines for junior workers             | -15% to -16% employment for early-career workers       |
| Hosseini Maasoum & Lichtinger 2025 | 62M workers/285,000 firms | "Seniority-biased technological change": AI substitutes junior labor               | Sharp decline in junior employment via slower hiring   |

### Workforce Impact

Effects on the workforce appear uneven, initially showing up in hiring pipelines, among younger workers, and within specific business functions. The evidence does not point to broad, uniform displacement. Firm-level survey data suggests many organizations expect the pace of workforce change to accelerate over the next year. One-third of respondents anticipate a decrease in workforce size (35% at organizations with ≥$1B revenue, 30% at smaller firms). Most respondents (43%) expect little or no change.

Employment data for software developers and customer service roles reveals a **generational pattern**. Employment among workers ages 22–25 has declined since 2022, even as headcount for older age groups continues to grow. By September 2025, employment for software developers ages 22–25 had fallen close to 20% from its 2022 peak. Among workers ages 22–25, employment in the most AI-exposed occupations has fallen roughly 16% relative to the least-exposed; the gap began widening in mid-2024 and has grown steadily since.

Unemployment data shows an even more complicated dynamic: from 2022 to early 2025, unemployment rose across all occupation groups regardless of AI exposure level. AI exposure alone does not seem to be driving recent unemployment trends, but it appears to play a part in broader macroeconomic conditions.

Over comparable time frames, the occupational mix in the United States has shifted **faster** since the introduction of generative AI than the shift that followed the introduction of computers or the internet.

A survey of 844 occupational tasks found that 46.1% of workers actively want AI to take over those tasks — particularly to free up time for higher-value tasks and reduce repetitiveness. However, actual usage patterns do not necessarily reflect these preferences: occupational tasks with the highest average automation scores account for only 1.3% of Claude.AI usage.

---

## 4.5 Robot Deployments

Physical automation through robotics represents one form of AI's economic integration in industrial environments. The AI Index uses data from the International Federation of Robotics (IFR 2025).

### Global Installation Patterns

Global industrial robot activity continues to rise, though year-over-year growth has flattened. In 2024, **542,000 industrial robots** were installed globally, a slight increase (0.2%) from the previous year. The total operational stock in 2024 grew to **4,664,000**, up from 4,282,000 in 2023. The global fleet of industrial robots has quadrupled since 2012. Collaborative robots, designed to work alongside human operators, continue to gain market share — from 2.8% of all new industrial robot installations in 2017 to 13.6% in 2024.

### Geographic Patterns

In 2024, **China led** the world with 295,000 industrial robot installations, six times more than Japan (44,500) and 8.6 times more than the United States (34,200). South Korea (30,600) and Germany (27,000) followed. China's share of global installations has increased substantially from 20.8% in 2013 to 54.4% in 2024. Taiwan showed the strongest annual growth (+33%), while Italy saw the steepest decline (-16%).

**Industrial robots installed by geography, 2024** *(Fig. 4.5.4)*

| Country      | Installs (thousands) |
| ------------ | -------------------- |
| 🇨🇳 China    |                  295 |
| 🇯🇵 Japan    |                 44.5 |
| 🇺🇸 US       |                 34.2 |
| 🇰🇷 S. Korea |                 30.6 |
| 🇩🇪 Germany  |                   27 |
| 🇮🇳 India    |                  9.1 |
| 🇮🇹 Italy    |                  8.8 |
| 🇹🇼 Taiwan   |                  5.8 |
| 🇲🇽 Mexico   |                  5.6 |
| 🇪🇸 Spain    |                  5.1 |

### Service Robotics

Nonindustrial service robots designed for logistics, hospitality, and agriculture showed growth in 2024. The number of service robots deployed in agriculture increased 2.5-fold. Only the hospitality category saw a year-over-year decline. Transportation and logistics remained the largest service robot category at 103,000 units in 2024.

---

# Chapter 5: Science

> **Overview:** The speed with which AI is transforming science is accelerating. In 2025, AI moved beyond improving individual pipeline steps and toward replacing entire scientific workflows, from weather prediction to multiagent hypothesis generation and experimental design. Still, rigorous benchmarks continue to expose large gaps between plausible output and reliable scientific work, with frontier agents scoring below 20% on paper-scale replication tasks. AI's impact in social sciences has been slower to emerge but with notable exceptions.

## Chapter 5 Highlights

1. **AI-related scientific publications are growing year-over-year.** Natural sciences reached approximately 80,150 AI publications in 2025, up 26% from 2024. AI now accounts for 5.8%–8.8% of scientific research output depending on the field, up from below 1% in 2010.
2. **Frontier models outperform human chemists on average but cannot reproduce published research.** On ChemBench, the best models surpass human expert averages across 2,700+ chemistry questions while struggling with basic tasks. On ReplicationBench, frontier models score below 20% on paper-scale replication in astrophysics.
3. **Astronomy released its first foundation model, first visualization benchmark, and a 100TB training dataset in 2025.** AION-1, trained on over 200 million celestial objects from 5 major surveys, is the first astronomy foundation model.
4. **An AI system ran a full weather forecasting pipeline end-to-end for the first time in 2025.** Aardvark Weather replaced the traditional numerical prediction pipeline with a single ML system. FourCastNet 3 generates a 60-day global forecast in under 4 minutes, running 8 to 60 times faster than prior approaches.
5. **On end-to-end scientific research tasks, the best AI agents score roughly half of what PhD experts achieve.** On PaperArena, the best agent reaches 38.8% accuracy versus a PhD expert baseline of 83.5%. On BixBench, frontier models achieve roughly 17% accuracy on real-world bioinformatics analysis.
6. **The first fully AI-generated paper was accepted at a peer-reviewed workshop in 2025, but the list of experimentally confirmed AI discoveries remains short.** Sakana's AI Scientist-v2 produced a paper accepted at an ICLR workshop without human-coded templates.
7. **Most AI models for science originate from academic and government institutions**, in contrast with the industry-dominated landscape of general-purpose AI.

---

## 5.1 AI for Science in 2025

AI's role in science falls into three categories: (1) machine learning over scientific data to build predictive models (now commonplace), (2) AI systems that assist scientists in their workflows (expanded considerably in 2025), and (3) autonomous AI systems capable of generating new scientific discoveries with limited human guidance (gaining traction but at an early stage). The clearest breakthroughs cluster in domains with strong existing data infrastructure: structural biology, physics, chemistry, and materials science.

### Publications in AI for Science

In the Web of Science database, AI-related publications in the natural sciences reached approximately **80,150 in 2025**, up from 63,547 in 2024 (+26%). Physical sciences and life sciences each grew ~27%–28% year over year. By 2025, Earth science had the highest AI penetration at 8.8%, followed by natural sciences overall at 6.8%, life sciences at 6.5%, and physical sciences at 5.8%.

---

## 5.2 AI Across Scientific Domains

### Physics, Astronomy, Chemistry, and Materials Science

AI is accelerating these fields by replacing expensive first-principles simulations with learned surrogates and by generating novel materials and molecular structures through inverse design. Notable 2025 releases include large chemistry datasets (OMol25, OC25), simulation-oriented foundation models (Walrus, GPhyT), and materials checkpoints for atomistic modeling and generation (MACE-MP-0, MatterGen). In chemistry and materials science, agent systems began connecting to external software tools and laboratory equipment to execute experiments. Benchmarks suggest these systems are not yet reliable when asked to carry out full research tasks from start to finish.

**Selected datasets in physics, astronomy, chemistry, and materials science (2025)** *(Fig. 5.2.1)*

| Name                     | Domain                          | Sector                          | Summary                                                                  |
| ------------------------ | ------------------------------- | ------------------------------- | ------------------------------------------------------------------------ |
| ChemPile                 | Chemistry                       | 🏫 Academia / 🔵 Nonprofit     | 75B+ tokens of curated chemical data (SMILES, InChI, text, code)        |
| Multimodal Universe      | Astronomy                       | 🏫 Academia / 🔵 Nonprofit     | 100TB astronomical dataset; multichannel images, spectra, time series   |
| Open Molecules 2025 (OMol25) | Chemistry, Materials, Chemical Physics | 🏭 Industry / 🏛️ Gov / 🏫 Academia | 100M+ DFT calculations spanning 83 elements, structures up to 350 atoms |
| Open Catalyst 2025 (OC25) | Chemistry, Materials           | 🏭 Industry / 🏛️ Gov / 🏫 Academia | 7.8M calculations across 1.5M solvent environments, 88 elements        |

**Selected benchmarks in physics, astronomy, chemistry, and materials science (2025)** *(Fig. 5.2.2)*

| Name                   | Domain                       | Sector        | Summary                                                                       |
| ---------------------- | ---------------------------- | ------------- | ----------------------------------------------------------------------------- |
| AstroVisBench          | Astronomy                    | 🏫/🏛️         | First benchmark for LLM scientific computing and visualization in astronomy   |
| ChemBench              | Chemistry                    | 🏫/🏭/🏛️      | 2,700+ Q&A pairs. Best models outperform human chemists but struggle with basics |
| ChemX                  | Chemistry, Materials Science | 🏫/🏭         | 10 curated datasets for chemical information extraction                        |
| GravityBench           | Physics, Astrophysics        | 🏫            | Tests AI discovery of physics laws from gravitational simulations              |
| LLM-SRBench            | Physics, Scientific Equation Discovery | 🏫/🏭 | 239 problems testing equation discovery vs. memorization; best: 31.5%      |
| MatSciBench            | Materials Science            | 🏫            | 1,340 college-level problems across 6 fields and 31 subfields; top models <80% |
| PHYBench               | Physics                      | 🏫            | 500 original physics problems. Gemini 2.5 Pro: 36.9% vs. human experts: 61.9% |
| ReplicationBench        | Astrophysics/Research Replication | 🏫       | Tests AI replication of entire astrophysics papers; frontier models score <20% |
| TPBench                | Theoretical Physics          | 🏫            | 57 novel theoretical physics problems (high-energy theory, cosmology); largely unsolved |

**Selected foundation models in physics, astronomy, chemistry, and materials science (2025)** *(Fig. 5.2.3)*

| Name           | Domain                         | Sector        | Summary                                                                    |
| -------------- | ------------------------------ | ------------- | -------------------------------------------------------------------------- |
| AION-1         | Astronomy                      | 🏫/🔵/🏛️     | 300M–3.1B params. 200M+ celestial objects from 5 major surveys. Open release |
| ChemDFM        | Chemistry                      | 🏫/🏛️/🏭     | Chemistry LLM: 34B tokens, 2.7M instructions. Generalist chemical AI       |
| GPhyT          | Physics                        | 🏫            | Trained on 1.8TB simulation data. Up to 29x better than specialized models |
| MACE-MP-0      | Chemical Physics               | 🏫/🏛️/🏭     | General-purpose force field model for nearly all materials                 |
| MatterGen      | Materials Science              | 🏭/🏛️         | Diffusion-based generative model. Over 2x more novel and stable            |
| PDE-Transformer | Physics Simulations           | 🏫            | Outperforms state-of-the-art vision architectures across 16 simulation types |
| PhysiX         | Physics Simulations            | 🏫            | 4.5B params. First large-scale physics simulation FM                        |
| SMI-TED        | Chemistry                      | 🏭            | Chemical foundation models trained on molecular sequences                   |
| Surya          | Heliophysics                   | 🏫/🏛️/🏭     | 366M params. First heliophysics FM. Forecasts space weather from NASA data |
| Walrus         | Fluid Mechanics / Multi-domain | 🏫/🔵/🏭      | Fluid mechanics FM covering astrophysics, geoscience, plasma, acoustics. Open weights |

**AI Agents in physics, astronomy, chemistry, and materials science (2025)** *(Fig. 5.2.4)*

Physics Supernova scored 23.5 out of 30 at the 2025 International Physics Olympiad, ranking 14th out of 406 participants at gold-medalist level. StarWhisper Telescope automates astronomical observation planning across 10 telescopes. ChemAgents demonstrated autonomous synthesis and optimization using a robotic platform controlled by Llama-3.1-70B.

| Name                  | Domain                    | Sector        | Summary                                                                           |
| --------------------- | ------------------------- | ------------- | --------------------------------------------------------------------------------- |
| ChatGPTMaterial Explorer | Materials Science      | 🏫            | Combines LLMs with graph neural networks for property prediction                  |
| ChemAgents            | Chemistry                 | 🏫/🏛️         | Robotic AI chemist (Llama-3.1-70B). Autonomous synthesis, optimization            |
| ChemToolAgent         | Chemistry, Materials      | 🏫/🏛️         | 137 external chemical tools. HE-MCTS framework surpasses GPT-4o on chemistry QA  |
| Crystalyse            | Materials Science, Chemistry | 🏫          | Multi-tool AI agent for materials design; LLM-based reasoning framework           |
| Physics Supernova     | Physics                   | 🏫            | IPhO 2025: 23.5 out of 30, ranked 14th of 406. Gold-medalist level                |
| StarWhisper Telescope | Astronomy                 | 🏫/🏛️/🏭     | Automates astronomical observations across 10 telescopes. LLM-driven              |

---

### Biological and Life Sciences

AI is increasingly being applied to biological research beyond biomedicine to address fundamental questions in genomics, neuroscience, ecology, and synthetic biology. The scale of biological training data grew in 2025, and foundation models trained on genomic and evolutionary data expanded from prediction into generative design. The gap between genomic sequence data (abundant) and functional perturbation data (scarce) remains wide. Computer vision and acoustic models routinely process sensor data to track species populations and optimize agricultural water use in real time. In neuroscience, AI serves both as a practical tool for brain mapping and as a source of theoretical inspiration.

**Selected datasets in biological and life sciences (2025)** *(Fig. 5.2.5)*

| Name          | Domain                     | Sector        | Summary                                                                       |
| ------------- | -------------------------- | ------------- | ----------------------------------------------------------------------------- |
| OpenGenome2   | Biology, Genomics          | 🏫/🔵/🏭     | 9.3T base pairs of curated DNA from all domains of life. Training corpus for Evo 2 |
| ProteinTalks-DB | Proteomics, Systems Biology | 🏫           | 38M+ proteomics measurements from drug-treated breast cancer cells            |
| Spacetop      | Neuroscience               | 🏫            | 101 participants, 600+ imaging hours. Cognitive, affective, social domains     |

**Selected benchmarks in biological and life sciences (2025)** *(Fig. 5.2.6)*

| Name                              | Domain              | Sector    | Summary                                                                           |
| --------------------------------- | ------------------- | --------- | --------------------------------------------------------------------------------- |
| BaisBench                         | Biology             | 🏫        | Evaluates AI biological discovery via cell annotation and data-driven questions   |
| BioML-bench                       | Biology             | 🏭/🏫     | First end-to-end biomedical ML evaluation. Agents underperform human baselines    |
| BixBench                          | Computational Biology | 🏭       | 50+ bioinformatics scenarios. GPT-4o and Claude 3.5 Sonnet: ~17% accuracy        |
| CGBench                           | Biology (Genetics)  | 🏫        | Clinical genetics interpretation. Reasoning models excel; hallucination gaps remain |
| Mouse vs. AI: Robust Foraging Competition | Neuroscience | 🏫  | Bioinspired RL benchmark grounding agents via shared foraging tasks with mice    |

**Selected foundation models in biological and life sciences (2025)** *(Fig. 5.2.7)*

| Name          | Domain                    | Sector        | Summary                                                                        |
| ------------- | ------------------------- | ------------- | ------------------------------------------------------------------------------ |
| AlphaGenome   | Biology, Genomics         | 🏭            | Genomic FM predicting thousands of functional measurements from DNA at single-base-pair resolution |
| ANN Model     | Neuroscience              | 🏫            | Neural activity FM. Predicts neuronal responses, generalizes across stimulus types and animals |
| BioCLIP 2     | Biology                   | 🏫/🔵         | Vision FM for biological classification across the tree of life                |
| BioLab        | Biology                   | 🏫/🏭         | Multiagent system for automated biological research. Experimentally validated novel antibody designs |
| CellFM        | Biology                   | 🏫/🏛️/🏭     | 800M params, 100M human cells. Single-cell analysis, perturbation prediction   |
| Evo 2         | Biology, Genomics         | 🏫/🏭         | 40B params, 1M token context. 9.3T base pairs. Genome-scale generation. Fully open release |
| ProteinTalks  | Proteomics, Systems Biology | 🏫/🏭        | Predicts drug efficacy and synergy from perturbation proteome data             |

**AI Agents in biological and life sciences (2025)** *(Fig. 5.2.8)*

Agent systems in the life sciences are beginning to operationalize complex research workflows, including literature synthesis and bioinformatics execution. BCI-Agent performs autonomous neuronal cell-type classification from electrophysiology recordings without task-specific training. Biomni is a general-purpose biomedical agent spanning 25 subfields.

| Name       | Domain       | Sector        | Summary                                                                       |
| ---------- | ------------ | ------------- | ----------------------------------------------------------------------------- |
| BCI-Agent  | Neuroscience | 🏫            | Autonomous neuronal cell-type classification from electrophysiology. No task-specific training |
| BioAgents  | Biology      | 🏫/🏭         | Multiagent system on small LMs with RAG. Expert-level on conceptual genomics tasks |
| Biomni     | Biology      | 🏫/🏭/🔵      | General-purpose biomedical agent across 25 fields                             |

---

### Earth Science

Progress in AI for Earth science remains aligned with observational infrastructure, including reanalysis datasets and global satellite archives. Weather forecasting has advanced furthest, with multiple AI models being used in real forecasting systems in 2025. Climate modeling lags behind because it requires projections on decadal timescales where future states fall outside training data. ClimateAgent completed 85 climate tasks with 100% completion and a quality score of 8.32, compared with 6.27 for Microsoft Copilot and 3.26 for GPT-5.

**Selected datasets in Earth science (2025)** *(Fig. 5.2.9)*

| Name         | Domain   | Sector    | Summary                                                                                |
| ------------ | -------- | --------- | -------------------------------------------------------------------------------------- |
| AmeriFlux    | Ecology  | 🏫/🏛️    | 260+ flux tower sites measuring ecosystem carbon, water, and energy exchange           |
| CAMELS       | Hydrology | 🏛️       | Standardized data on terrain, climate, soil, and streamflow for 671 U.S. river basins  |
| FLUXNET2015  | Ecology  | 🏫/🏛️    | Global CO2, water, and energy exchange from 212 sites worldwide                        |
| ICOS         | Ecology  | 🏫/🏛️    | European 140+ station network measuring greenhouse gas concentrations across 12 countries |
| JapanFlux    | Ecology  | 🏫/🏛️    | Land-atmosphere flux measurements covering Japan and East Asia from 1990 to 2023       |

**Selected benchmarks in Earth science (2025)** *(Fig. 5.2.10)*

| Name       | Domain              | Sector    | Summary                                                                     |
| ---------- | ------------------- | --------- | --------------------------------------------------------------------------- |
| EarthSE    | Earth Science       | 🏫/🏭     | 100K papers, 114 disciplines, 11 LLMs tested. Significant gaps in Earth science exploration |
| ExEBench   | Atmospheric Sciences | 🏫        | 7 extreme event categories. Tests detection, monitoring, and forecasting    |
| UnivEARTH  | Earth Science       | 🏫        | 140 Earth observation questions. LLM agents: 33% accuracy. Code fails 58%  |

**Selected foundation models in Earth science (2025)** *(Fig. 5.2.11)*

| Name          | Domain              | Sector        | Summary                                                                     |
| ------------- | ------------------- | ------------- | --------------------------------------------------------------------------- |
| AlphaEarth    | Earth observation   | 🏭            | Embedding field model for general geospatial representation                 |
| cBottle (Climate in a Bottle) | Climate Science | 🏭  | Diffusion-based climate emulator. Global 5km at 12.5M-pixel resolution     |
| FourCastNet 3 | Weather Forecasting | 🏭/🏛️/🏫     | 60-day forecast in <4 min/GPU. 8–60× faster. Builds on Aurora and NeuralGCM |
| GAIA          | Atmospheric Sciences | 🏭/🔵         | Atmospheric FM from 15 years of satellite imagery. Cyclone detection (81% recall) |
| OlmoEarth     | Earth Observation   | 🔵/🏫         | Multimodal spatiotemporal Earth observation FM for NGOs and nonprofits      |
| TerraMind     | Earth Observation   | 🏭/🏫/🏛️     | First any-to-any generative multimodal Earth observation FM. 9 geospatial modalities |
| WeatherNext 2 | Weather Forecasting | 🏭            | Hundreds of weather outcomes in <1 min/TPU. 99.9% improvement over predecessor |

**AI Agents in Earth science (2025)** *(Fig. 5.2.12)*

| Name          | Domain         | Sector | Summary                                                                             |
| ------------- | -------------- | ------ | ----------------------------------------------------------------------------------- |
| ClimateAgent  | Climate Science | 🏫    | 85 climate tasks: 100% completion, quality 8.32 vs. Copilot 6.27, GPT-5 3.26       |
| EarthLink     | Climate Science | 🏭/🏫  | First AI copilot for Earth scientists. Automated research workflows with feedback loop |
| PANGAEA GPT   | Earth Science  | 🏛️    | Multi-agent system for PANGAEA Earth science database. Intelligent data processing  |

---

### Mathematics

Mathematical reasoning is another active testing ground for AI capabilities. Systems such as Goedel-Prover are moving toward automated formal proof generation in languages like Lean. Competition-level problem-solving and formal verification of known results are advancing quickly, but major open problems, such as long-standing Erdős conjectures, remain well beyond current capabilities. Chapter 2 covers benchmark performance in detail, including a jump from silver to gold medal at the International Mathematical Olympiad in a single year and rapid gains on FrontMath and MathArena.

---

## 5.3 AI Agents and Tools for Science Workflows

The domain-specific tables of Section 5.2 catalog a growing inventory of AI agents, foundation models, datasets, and benchmark suites. Two cross-domain benchmarks released in 2025 offer a broader view of how well these systems perform when asked to do end-to-end scientific research rather than isolated tasks. On both benchmarks, even the best-performing agents fall well below expert-level performance.

### AstaBench

AstaBench is an end-to-end benchmark suite that evaluates agentic scientific research ability across over 2,400 problems spanning multiple domains and the full discovery workflow, from literature understanding through code execution, data analysis, and end-to-end discovery. It benchmarked 57 agents across 22 agent classes. The best performing agent scored around **0.53** at a cost of roughly $3.40 per problem, while most agents clustered between 0.10 and 0.45 at per-problem costs below $1.00.

### PaperArena

PaperArena tests whether LLM agents can answer real research questions that require stitching together evidence across multiple papers while orchestrating external tools for parsing, retrieval, and computation. Gemini 2.5 Pro performs best overall, achieving **38.8%** average accuracy in a multiagent configuration. All tested agents lagged substantially behind the PhD expert baseline of **83.50%**. Multiagent configurations consistently outperformed single-agent setups, typically by 2 to 4 percentage points.

### AI as a Co-scientist

In 2025, several research groups released systems in which multiple AI agents divide scientific tasks among themselves, with separate agents handling literature search, hypothesis generation, code execution, and review. The most prominent example, **Google's AI Co-scientist** (Gottweis et al., 2025), uses a generate-debate-evolve loop in which agents iteratively produce and refine evidence-grounded hypotheses. It was validated in three biomedical areas, including AML drug repurposing and liver fibrosis targets, achieving a top-1 accuracy of 78.4% on GPQA Diamond when selecting its highest-rated hypothesis per question.

Other multiagent systems:
- **Sakana's AI Scientist-v2** produced the first fully AI-generated paper accepted at a peer-reviewed workshop (ICLR), using agentic tree search to generate and refine code without human-coded templates
- **Kosmos** maintained coherence across runs lasting up to 12 hours, executing ~42,000 lines of code and reading 1,500 papers per run; collaborators reported a single run approximated six months of research
- **SciToolAgent** automates hundreds of scientific tools across biology, chemistry, and materials science via knowledge-graph-driven retrieval, outperforming prior agent frameworks by 10 to 20 percentage points on multi-tool tasks

Despite these advances, only a handful of multiagent systems have produced results that were tested and confirmed through real-world experiments. The gap between what these systems can propose computationally and what has been confirmed experimentally remains wide. Key roadblocks include workforce training gaps, a lack of API and interoperability standards, and funding structures that do not yet support the maintenance and scaling of autonomous research infrastructure.

---

# Chapter 6: Medicine

> **Overview:** AI in medicine advanced on multiple fronts in 2025, but strong model performance has not consistently translated into real-world clinical impact. In molecular biology, AI-driven protein research continued to grow, and smaller, more specialized models matched or outperformed larger general-purpose systems on protein structure prediction, genomics, and drug discovery. On clinical reasoning tasks, leading AI models now score higher than most physicians on structured clinical evaluations, yet nearly half of clinical AI studies still rely on simulated scenarios rather than real patient data. The tools gaining traction in practice are those that support clinicians' existing workflows, such as ambient AI scribes and sepsis prediction systems. Authorizations from the U.S. FDA for AI-enabled medical devices increased, but clinical evidence continues to lag behind. AI's impact on medicine is clear, but realizing it at scale will require clinical evidence, governance, and ethical frameworks.

## Chapter 6 Highlights

1. **In molecular biology, smaller models are outperforming larger ones.** MSAPairformer, a 111-million-parameter protein language model, outperformed previous leading methods on ProteinGym; and GPN-Star, a 200-million-parameter genomics model, outperformed a model with 40 billion parameters.
2. **Virtual cell models emerged as a new frontier in 2025**, with major releases including Evo 2, STATE, and DeepMind's AlphaGenome. These models aim to predict cellular responses to drugs and genetic perturbations without running wet-lab experiments, though current systems still require experimental validation.
3. **Like other areas of AI, biological model development is increasingly bottlenecked on data rather than architecture.** With cofolding models now representing all structure types in the Protein Data Bank, 2025 saw a turn toward distilled datasets of AI-predicted structures and training on combined experimental data sources.
4. **AI tools that automatically generate clinical notes from patient visits saw broad adoption in 2025.** Across multiple hospital systems, physicians reported they were spending up to 83% less time writing notes, experiencing significant reductions in burnout, with one hospital system reporting a 112% return on investment.
5. **The FDA authorized 258 AI medical devices in 2025**, most through pathways that do not require new clinical trials. Only 2.4% of devices had clinical studies supported by randomized trial data.
6. **A multi-agent AI system scored 85.5% on complex published case studies, versus 20% for unaided physicians.** Microsoft's AI Diagnostic Orchestrator, paired with OpenAI's o3, was tested on challenging cases drawn from the medical literature.
7. **AI-generated summaries now appear at the top of 84% to 92% of health-related Google searches.** Symptom and common health questions trigger an AI Overview 92% of the time.
8. **Ethics discussion in medical AI publications more than doubled in 2025, but the conversation is narrow.** Governance dominates the discourse, while algorithm accountability, biosecurity, and global health equity remain underexplored.
9. **Research interest in medical digital twins is growing fast, and where rigorous trials exist, early results are promising.** In a randomized trial of 150 diabetes patients, 71% achieved healthy blood sugar levels over one year while safely reducing their medications.

---

## 6.1 The Central Dogma

AI models for molecular biology span the pathway from gene sequence to protein structure to therapeutic design. A recurring pattern across these areas is the tension between scale and specialization — in several areas, smaller or more targeted models matched or outperformed larger general-purpose systems.

### Research Volume

AI-driven protein research grew approximately **71% between 2024 and 2025**, rising from 2,259 to 3,855 total publications across four categories: function prediction, protein structure prediction, protein-drug interactions, and synthetic protein design. Protein-drug interactions represented the largest share: 49.9% of papers in 2024, rising to 54.4% in 2025. Publications on AI for drug discovery reached **3,311 in 2025**, up from 2,100 in 2024 (a continuing steep upward trajectory since 431 in 2018).

**AI-driven protein research publications, 2024 vs. 2025** *(Fig. 6.1.1)*

| Research domain          | 2024  | 2025  |
| ------------------------ | ----- | ----- |
| Function prediction      |   220 |   402 |
| Protein structure prediction |  648 |  922 |
| Protein-drug interactions | 1,127 | 2,097 |
| Synthetic protein design |   254 |   434 |

### Public Datasets — Molecular and Cellular Biology

Demand for training data has continued to grow as AI models have gained further adoption in biology. Several cofolding methods began training on both structural data from the Protein Data Bank (PDB) and experimental small-molecule binding affinity measurements from repositories such as PubChem, ChEMBL, and BindingDB. Meta FAIR released Open Molecules 2025 (OMol25), a dataset of over 100 million quantum mechanics calculations of molecules. New experimental datasets in 2025 include Tahoe-100M, the largest publicly released single-cell sequencing dataset, with measurements from over 50 cancer cell types exposed to more than 1,100 drugs, and BaseData, which features over 9.8 billion genes obtained through metagenomic mining.

---

### Data for Biomedical Vision-Language Models

Training biomedical vision-language models requires large repositories of images and captions. In the general domain, data scaling is often considered a mature or saturated direction, but this does not appear to hold in the biomedical setting. Newer datasets extend beyond a single specialty and incorporate broader modalities. Key biomedical multimodal datasets include MEDICAT (2020), PMC-OA (~2.2M, 2022), PMC-15M (~3M, 2024), and BIOMEDICA 24M (~5.7M, 2025).

### Sequence-Based Models: Protein Language Models

The trend in protein language models (PLMs) shifted in 2025 from scaling model size to improving model efficiency and specialization. In 2024, efforts culminated in the 98-billion-parameter ESM3. In 2025, the focus turned to smaller architectures trained on curated data or augmented with retrieval methods. **MSAPairformer**, a 111-million-parameter model, surpassed previous state-of-the-art methods on ProteinGym at a fraction of the training and parameter budget. The **Profluent E1** series set new performance standards by combining a smaller model with a RAG approach. PLMs have also become more task-specific — a fine-tuned ProGen model (6B parameters) was used to design a novel CRISPR-Cas protein, OpenCRISPR-1.

**ProteinGym benchmark performance, 2021–25** *(Fig. 6.1.6)*

| Model                          | Year | Avg Correlation (Spearman) |
| ------------------------------ | ---- | -------------------------- |
| MSA-Transformer (100M)         | 2021 |                       0.45 |
| ESM-2 (650M)                   | 2022 |                       0.41 |
| PaET (200M)                    | 2023 |                       0.47 |
| ESMC (600M)                    | 2024 |                       0.41 |
| MSA-Pairformer (111M)          | 2025 |                       0.45 |
| E1 (Retrieval Augmented, 600M) | 2025 |                       0.48 |

### Structure Prediction and Cofolding Models

Multiple open-source structure prediction models were released in 2025, inspired by AlphaFold 3's architecture. These models tackle "cofolding" — predicting 3D structures of proteins, nucleic acids, drugs, and other biomolecules in combination. While AlphaFold 3 retains a performance advantage on certain tasks, most cofolding models have demonstrated similar performance. Some, including the Boltz series and OpenFold3, are released under commercially permissive licenses.

**Training data sources for cofolding models, 2025** *(Fig. 6.1.6)*

| Model        | Structural (exp.) | Structural (distilled) | Molecular dynamics | Binding affinity | RNA structure |
| ------------ | :---------------: | :--------------------: | :----------------: | :--------------: | :-----------: |
| AlphaFold 3  | ✅                | ✅                     |                    | ✅               |               |
| Boltz-2      | ✅                | ✅                     | ✅                 | ✅               |               |
| SimpleFold   | ✅                | ✅                     | ✅                 |                  |               |
| OpenFold3    | ✅                | ✅                     |                    | ✅               | ✅            |

Following the release of AlphaFold 3, subsequent models have converged on a similar parameter scale rather than continuing to grow. **FoldBench** tests whether a model can correctly predict how a small molecule physically binds to a target protein. AlphaFold 3's performance on FoldBench has yet to be significantly surpassed even though several larger models have been released since, suggesting that data rather than model size is the key bottleneck.

**FoldBench: protein cofolding performance, 2024–25** *(Fig. 6.1.9)*

| Model                    | Year | Accuracy |
| ------------------------ | ---- | -------- |
| AlphaFold 3 (370M)       | 2024 |  64.90%  |
| Boltz-1 (610M)           | 2024 |  55.04%  |
| SeedFold (923M)          | 2025 |  63.10%  |
| RosettaFold-3 (730M)     | 2025 |  57.28%  |
| Boltz-2 (521M)           | 2025 |  53.90%  |

### Protein Design and Generative Models for Therapeutics

Advances in cofolding have enabled a new generation of generative models for protein design, including methods for designing antibodies, nanobodies, and peptides. A protein design challenge hosted by Adaptyv Bio in 2025 provided a controlled comparison. Multiple methods were tested on designing a binder targeting Nipah virus. Of the 1,026 designs tested, 99 proteins were confirmed to bind, and none neutralized the targeted protein. The specialized method Mosaic (100% expressed, 95.89% bound) significantly outperformed general-purpose approaches like BoltzGen (98% expressed, 1.87% bound) and RFDiffusion (88% expressed, 17.65% bound).

### Virtual Cell Models and Genomic Foundation Models

Research on "virtual cell" models increased substantially in 2025, with publications rising from 16 (2024) to 24. Notable releases included Evo 2 (Arc Institute), STATE (a perturbation-response model), and AlphaGenome (DeepMind). However, current virtual cell and genomic foundation models still lag behind smaller, task-specific models. GPN-Star, a 200-million-parameter model focused on functional and regulatory genomics, outperformed Evo 2 (40B parameters) on multiple variant effect prediction tasks, suggesting scale alone is not yet sufficient.

**Virtual cell model performance (AUPRC)** *(Fig. 6.1.12)*

| Model            | Year | AUPRC |
| ---------------- | ---- | ----- |
| Enformer (250M)  | 2021 |  0.38 |
| Borzoi (190M)    | 2023 |  0.40 |
| Evo 2 (40B)      | 2025 |  0.53 |
| GPN-Star (200M)  | 2025 |  0.75 |

### Multimodal Foundation Models for Biomedical Discovery

Scientific publications on multimodal foundation models for biomedical discovery have been growing rapidly since 2021 (2 publications in 2021 → 462 in 2025). Two subfields have been especially impactful: vision-language models (pair medical or biological images with text) and vision-omics models (integrate imaging with genomic or transcriptomic data).

> **HIGHLIGHT: Automated and Agentic Biomedical Discovery**
>
> In 2025, efforts to automate scientific discovery focused on integrating digital reasoning with physical laboratory validation. **Robin**, an automated discovery framework, linked literature-based hypothesis generation with experimental data analysis, identifying the ROCK inhibitor ripasudil as a novel candidate for dry age-related macular degeneration. **STELLA**, an autonomous bioinformatics agent, expanded its own technical capabilities by discovering and integrating new software tools rather than relying on manually curated toolsets. **Biomni**, developed at Stanford, mapped a unified biomedical action space across 25 subfields, integrating 150 specialized tools, 105 software packages, and 59 databases. **The Virtual Lab** uses an LLM Principal Investigator to orchestrate specialized scientist agents, producing 92 novel nanobody binder designs for SARS-CoV-2.

---

## 6.2 Clinical Applications

### Imaging

#### Data Scale and Availability

Training data for medical imaging AI remains roughly 100 times smaller in raw sample count than for nonmedical AI. MAIRA-2, a radiology-focused model, trained on approximately 1.4 million chest radiographs compared with DINOv3, a general-purpose vision transformer trained on 1.7 billion images. Data scarcity is especially pronounced for three-dimensional modalities such as CT and MRI.

#### Modeling Approaches

Vision language models (VLMs) for medical imaging have expanded beyond radiology into pathology, dermatology, ophthalmology, and cardiology. Across six clinical disciplines, the number of research models and FDA-cleared commercial products grew in 2025, with pathology seeing the greatest concentration of new research releases. The **Merlin** model demonstrated that a highly capable CT foundation model could be trained on a single 40GB GPU using radiology reports and ICD codes.

**Notable medical imaging model releases and FDA-cleared analogues, 2025** *(Fig. 6.2.2)*

| Discipline    | Notable releases                                    | FDA-cleared analogues                          |
| ------------- | --------------------------------------------------- | ---------------------------------------------- |
| Cardiology    | EchoJEPA, PanEcho, EchoFM, EchoPrime               | Bunkerhill ECG-EF, Heartflow Plaque Analysis   |
| Oncology      | MUSK                                                | Allix5, Clairity, Transpara 2.1.0              |
| Ophthalmology | EyeCLIP, Meta-EyeFM, RETFound-Green                 | CLARUS (700), Carl Zeiss                       |
| Pathology     | Virchow2G, KRONOS, VORTEX, Threads, mSTAR, MPath   | ArteraAI Prostate, Ibex Prostate Detect        |
| Radiology     | MedGemma 1.5, COLIPRI, RadFM, Merlin, CT-FM        | BriefCase Triage, a2z-Unified-Triage, Bunkerhill BMD, Brainomix 360 Triage Stroke |

#### Prospective Clinical Trials

The number of prospective trials validating medical imaging AI models grew by 28.5% year over year, from 417 in 2024 to **536 in 2025**. Recent trials include MASAI (randomized screening accuracy study of AI-assisted mammography) and NOTIFY-1/NOTIFY-EXTEND (tested whether AI flagging of early signs of heart disease on routine CT scans led doctors to prescribe more preventive cholesterol medication).

> **HIGHLIGHT: LLM Clinical Reasoning Performance**
>
> OpenAI's o1-preview was tested on diagnostic reasoning tasks, management reasoning vignettes, probabilistic reasoning scenarios, and real emergency department cases with blinded expert scoring. On NEJM clinicopathological conferences (n=143), the model included the correct diagnosis in its differential 78% of the time, with 52% top-1 accuracy. On NEJM Healer cases (80 responses), it achieved a perfect revised-IDEA score in 78 of 80, compared with 47 of 80 for GPT-4, 28 for attending physicians, and 16 for residents. On management reasoning, o1-preview's median score was **86%**, versus 42% for GPT-4 only, 41% for physicians with access to GPT-4, and 34% for physicians with conventional resources. These results suggest that current LLMs have surpassed most existing clinical reasoning benchmarks, but reflect isolated cognitive evaluations rather than real-world clinical integration.

> **HIGHLIGHT: AI Agents in Clinical Medicine**
>
> Autonomous and semiautonomous AI agents have emerged as a major development in clinical AI in 2025–26. Multiagent frameworks have shown early promise on benchmark evaluations. Diagnostic accuracy gains over single-agent baselines ranged from 7% to over 60%, depending on the complexity of the clinical task. **Microsoft's AI Diagnostic Orchestrator (MAI-DxO)**, paired with OpenAI's o3 reasoning model, achieved **85.5% accuracy** on diagnostically challenging cases from the New England Journal of Medicine, compared with approximately 20% among 21 practicing physicians working under comparable conditions. On MedAgentBench, which evaluates LLM agents in a virtual EHR environment across 300 clinically derived tasks, the best performing model achieved a task success rate of 69.7%.

### Deployment, Implementation, and Deimplementation

#### FDA-Authorized AI/ML-Enabled Devices

FDA 510(k)-cleared AI/ML-related devices reached **246 in 2025**, continuing a steep upward trajectory that began with 16 devices in 2016. By December 2025, the FDA had authorized a total of **1,357 AI/ML-enabled medical devices** from 693 different companies across 17 clinical specialties, crossing the 1,000-device milestone in 2024. Annual authorizations reached 258 through September 2025, already surpassing all prior full-year totals. 98 new companies entered the space in 2025.

**FDA AI medical devices by specialty, 1995–2025 (cumulative)** *(Fig. 6.2.7)*

| Specialty                     | Cumulative devices |
| ----------------------------- | ------------------ |
| 🔵 Radiology                  |              1,039 |
| 🩺 Cardiovascular             |                130 |
| 🧠 Neurology                  |                 61 |
| Anesthesiology                |                 23 |
| Gastroenterology-Urology      |                 21 |
| Hematology                    |                 20 |
| Ophthalmic                    |                 10 |

**Top FDA AI device companies, 2016–25 (cumulative)** *(Fig. 6.2.9)*

| Company                          | Devices |
| -------------------------------- | ------- |
| GE Healthcare                    |      93 |
| Siemens Healthineers             |      82 |
| Shanghai United Imaging Healthcare |    38 |
| Philips Healthcare               |      36 |
| Canon Medical Systems            |      35 |
| Aidoc Medical                    |      30 |

Despite growth, only **2.4%** of devices with clinical studies were supported by randomized controlled trial data.

#### Enterprise-Scale Deployments in 2025

**Ambient AI Documentation:** Ambient AI scribes saw the broadest adoption of any clinical AI category in 2025. Abridge expanded from ~100 to over 150 health systems, including Kaiser Permanente's deployment across 40 hospitals and 600+ medical offices. Adoption reached 63% among hospitals using Epic's EHR. Key outcomes:
- Sharp HealthCare: 83% reduction in note-writing effort; +3.5–6% increase in physician productivity per encounter
- University of Chicago Medicine: 47% reduction in cognitive load, 58% increase in undivided patient attention, 23% reduction in time spent on clinical notes
- Northwestern Medicine: 11.3 additional patients per month, 24% reduction in documentation time, 112% ROI
- Stanford Health Care: median 20-minute time savings per half-day of clinic; statistically significant reductions in task load and burnout

**AI-Powered Sepsis Prediction:** Two sepsis prediction systems reported mortality reductions in large-scale deployments. The Targeted Real-time Early Warning System, deployed across 13 Cleveland Clinic hospitals: 18.7% relative reduction in sepsis mortality, 1.85-hour reduction in median time to first antibiotic order, correct identification of 82% of sepsis cases. COMPOSER at UC San Diego Health: 17% reduction in sepsis mortality (1.9% absolute) across 6,217 admissions, 5% increase in sepsis bundle compliance, ~50 lives saved annually.

**Generative AI in Clinical Workflows:** Health systems began embedding LLM-powered tools into EHRs. ChatEHR logged 23,000 sessions across 1,075 trained users within three months of broad rollout. OpenEvidence, a real-time evidence retrieval platform, reported adoption by 40% of U.S. physicians.

#### Evidence Gaps and Governance

The inaugural State of Clinical AI Report (Stanford-Harvard ARISE Network, January 2026) reviewed over 500 clinical AI studies and found that nearly half used exam-style questions rather than real patient data; only 5% used real clinical data. The NOHARM benchmark found that leading LLMs produced 11.8 to 14.6 severely harmful recommendations per 100 clinical cases. Stanford Health Care's FURM framework now governs all new AI tool adoptions at that institution.

> **HIGHLIGHT: Digital Twins in Medicine**
>
> Publication counts on medical digital twins rose from near 0 in 2015 to **372 in 2025**. Patent filings increased from 30 in 2016 to 4,926 in 2025. However, only 12.1% (18 studies) of 149 human digital twin studies published between 2017 and 2024 satisfied the National Academies' definition of a digital twin (personalization, dynamic updating, and predictive capability). In a randomized controlled trial (n=150) of Twin Health's Whole Body Digital Twin platform, **71% of participants** achieved healthy blood sugar levels (HbA1c below 6.5%) within twelve months while safely reducing their medications.

---

## 6.3 Patient Engagement

### AI Overviews for Health-Related Searches

AI-generated summary responses now appear at the top of most health-related search results. On average, **84%–92%** of health-related queries triggered an AI Overview across five primary query types:

| Query type                         | AI Overview rate |
| ---------------------------------- | :--------------: |
| Common health queries              |             92%  |
| Symptom queries                    |             92%  |
| Treatment queries                  |             90%  |
| Condition question queries         |             88%  |
| Condition queries                  |             84%  |

### Patient Perspectives on AI in Healthcare

Publication volume on the patient perspective of AI in healthcare grew tenfold between 2020 and 2025 (9 → 102 publications). Conditional acceptance emerged as a prevalent perspective — patients tend to endorse AI in assistive roles rather than autonomous decision-making, particularly in high-stakes clinical contexts. Preservation of the human relationship and empathic care emerged as consistent primary concerns. Trust in AI appeared to be clinician-mediated rather than technology-evaluated: provider endorsement was a key determinant of patient acceptance. Demographic disparities in acceptance were documented across multiple studies (age, gender, education, and race). Studies from sub-Saharan Africa, Latin America, and Southeast Asia remain underrepresented.

---

## 6.4 Ethical Considerations

### Volume and Concentration

Of the total number of medical AI publications in 2025, **43.4%** discussed ethics topics — up from 37.1% in 2024. The absolute number more than doubled between the two years. Among specific topics, the growth was concentrated on governance (1,228 publications), outpacing algorithmic concerns (896) and societal concerns (874). Despite the attention paid to biosecurity in policy discussions, only 14 medical AI publications in 2025 discussed biosecurity.

**Medical AI ethics publications by topic, 2021–25** *(Fig. 6.4.2)*

| Year | Algorithmic | Governance | Societal |
| ---- | ----------- | ---------- | -------- |
| 2021 |         107 |        130 |       72 |
| 2022 |         199 |        170 |      121 |
| 2023 |         241 |        278 |      182 |
| 2024 |         471 |        544 |      347 |
| 2025 |         896 |      1,228 |      874 |

### Global Health: A Different Ethical Focus

Global health is an exception to the governance-dominated pattern. Among publications addressing global health in 2025, 51.8% (100 of 193) also mentioned ethics topics. In a departure from every other subcategory examined, **societal concerns** — including equity, justice, and accessibility — ranked highest in the global health context, surpassing both governance and algorithmic concerns. Europe led with 38 publications, followed by East Asia (31) and North America (28).

---

# Chapter 7: Education

> **Overview:** Demand for AI education is growing across every level, but the systems needed to deliver it are still catching up. Computer science enrollment in post-secondary institutions is declining even as AI-related majors gain popularity. Students at both the university and K-12 levels are using AI tools in large numbers, yet access to AI-specific coursework and teacher training remain limited. Governments are pushing to integrate AI literacy into their curricula to maintain competitive edge.

## Chapter 7 Highlights

1. **CS enrollment fell 11% at U.S. four-year universities between 2024 and 2025, but AI-related graduate programs continued to grow.** Master's graduates in AI software-related fields rose 17% from 2023 to 2024.
2. **The U.S. remains a global leader in producing ICT graduates at all degree levels, but other countries are growing faster.** Turkey, Brazil, and Mexico have increased their ICT graduate output more rapidly in recent years.
3. **Four out of five U.S. high school and college students now use AI for schoolwork, but school policies have not kept pace.** Only half of middle and high schools have AI policies, and just 6% of teachers say those policies are clear.
4. **More than 90% of countries now offer computer science to primary or secondary students, but AI education has been slower to take hold.** China and the UAE both mandated AI education starting with the 2025-26 school year.
5. **The number of new AI PhDs in the United States and Canada increased 22% from 2022 to 2024, but the share going to industry has stayed flat.** All of the growth has gone to academia, reversing a decade-long trend.
6. **People are acquiring AI skills outside formal education.** AI literacy has grown faster than engineering-oriented AI skills in most countries. The UAE, Chile, and South Africa are exceptions, where engineering skills show steeper growth since 2022.

---

## 7.1 Background

AI's role in education is expanding faster than the data needed to track it. Three distinct categories need to be distinguished:
- **AI in Education** — the usage of AI tools in teaching and learning
- **AI Literacy** — the foundational understanding of AI, how it works, how to use it, and the risks of usage
- **AI Education** — AI literacy plus the technical skills required to build AI systems

---

## 7.2 Postsecondary CS and AI Education

The generative AI usage among students has reshaped the conversation about the purpose and role of postsecondary education. Task automation in coding roles has appeared to slow the entry-level job market for CS graduates. Between 2024 and 2025, enrollment in CS as an undergraduate major in four-year universities **declined 11%**. Even as CS enrollment declines, there is evidence that AI-related majors are becoming more popular.

### U.S. Degree Graduates

AI software-related degrees (including Artificial Intelligence, Computer Programming, Computational and Applied Mathematics) have steadily increased in popularity over the past 10 years, especially at the bachelor's and master's levels. The largest increase has been at the master's level, with an **82% increase** in graduates between 2022 and 2024 and a 17% increase between 2023 and 2024. AI hardware-related degrees (Electrical and Electronics Engineering, Materials Physics, Industrial Engineering) have remained flat or declined.

**New AI-related postsecondary graduates, U.S., 2024** *(Fig. 7.2.1)*

| Category              | Level      | 2024 graduates (thousands) |
| --------------------- | ---------- | :-----------------------: |
| AI software-related   | Master's   |                    120.95 |
| AI software-related   | Bachelor's |                     94.92 |
| AI software-related   | PhD        |                     23.44 |
| AI software-related   | Associate's|                      6.03 |
| AI hardware-related   | Bachelor's |                     95.35 |
| AI hardware-related   | Master's   |                     43.37 |
| AI hardware-related   | PhD        |                      9.55 |
| AI hardware-related   | Associate's|                     18.75 |

Women remain underrepresented across AI-related degrees, peaking at 36% of AI software-related master's degree graduates and 27% of bachelor's graduates, while women account for nearly 60% of all degrees overall. Hispanic/Latino, Black, Native Hawaiian/Pacific Islander, and Native American/Alaskan students are underrepresented at all AI software-related degree levels.

The majority of AI-related graduate students are non-United States residents — 67% of AI software-related master's graduates and 55% of PhDs are nonresidents. Due to the federal government revoking student visas and discouraging international student enrollment, further declines in the number of nonresident graduates are expected in coming years.

**Top postsecondary institutions for AI-software related degrees, 2024** *(Fig. 7.2.6)*

| Rank | Bachelor's                                      | Master's                       | PhD                                |
| ---- | ----------------------------------------------- | ------------------------------ | ---------------------------------- |
| 1    | Univ. of Maryland Global Campus: 2,350          | Georgia Institute of Tech: 3,394 | Georgia Institute of Tech: 155    |
| 2    | Western Governors University: 2,029             | Univ. of Texas, Dallas: 2,555  | MIT: 154                           |
| 3    | UC Berkeley: 1,983                              | Columbia University: 2,481     | Univ. of Illinois, U-C: 128        |
| 4    | Univ. of Maryland, College Park: 1,877          | Univ. of North Texas: 2,454    | Carnegie Mellon University: 128    |
| 5    | Southern New Hampshire University: 1,733        | Trine University: 2,114        | Univ. of Michigan, Ann Arbor: 106  |

**AI PhD employment, 2024** *(Fig. 7.2.8)*

- **Industry:** 62.75% (down from peak of 77% in 2022)
- **Academia:** 31.59% (nearly doubled since 2022)
- **Government:** 1.96%

This challenges the narrative that academia is experiencing a brain drain — all of the growth in AI PhDs has gone to academia, reversing a decade-long trend.

### Global ICT Graduates

The United States remains the global leader in ICT-related fields, producing more graduates at the associate's, bachelor's, master's, and PhD levels than any other country in the OECD sample. At most levels, other countries had faster year-over-year growth. At the bachelor's level, both Brazil and Turkey increased their graduates by 30%; at the PhD level, Mexico increased its graduates by 76%. Turkey increased associate's-level graduates by 27%.

**New ICT bachelor's graduates by country, 2022–23** *(Fig. 7.2.10)*

| Country        | 2022    | 2023    |
| -------------- | ------- | ------- |
| United States  | 116,461 | 122,814 |
| Brazil         |  63,769 |  80,316 |
| Mexico         |  32,736 |  33,861 |
| Germany        |  27,505 |  21,504 |
| United Kingdom |     —   |  20,703 |

---

**New ICT master's graduates by country, 2022–23** *(Fig. 7.2.11)*

| Country        | 2023   |
| -------------- | ------ |
| United States  | 86,301 |
| United Kingdom | 27,624 |
| France         | 15,233 |
| Germany        | 12,650 |
| Australia      |  8,895 |
| Poland         |  4,571 |

**New ICT PhD graduates by country, 2022–23** *(Fig. 7.2.12)*

| Country        | 2023  |
| -------------- | ----- |
| United States  | 2,874 |
| United Kingdom | 1,218 |
| Germany        | 1,004 |
| France         |   830 |
| Korea          |   585 |

**Gender parity in ICT graduates:** On average, women account for 20% of associate's graduates, 22% of bachelor's graduates, 29% of master's graduates, and 29% of PhD graduates. Peru is an exception at the associate's level (~50% female); Costa Rica and Latvia approach parity at the PhD level.

### CS, CE, and Information Faculty

In 2024–25, there were over **6,600 CS, CE, and information faculty** in the United States and Canada. Nearly two-thirds filled tenure-track positions. CRA projections suggest the number will increase over the next two academic years, reaching 7,501 by 2026–27. Hispanic/Latino, Black, and Indigenous people are underrepresented in faculty positions, as are all women except Asian women. Asian and Native Hawaiian/Pacific Islander men are overrepresented among faculty.

### Student Use of AI Tools

In Chegg's 2025 survey of university students from 15 countries, **80% said they have used generative AI** to support their learning — double the share reported in 2023 when only 40% had. Generative AI usage varies widely by country: 95% of Indonesian students have used it, compared to 67% in the United States and United Kingdom. Students who use generative AI for school do so frequently: 56% input a question at least once a day.

**Top uses of generative AI for university schoolwork, 2025** *(Fig. 7.2.17)*

| Use                                        | % of students |
| ------------------------------------------ | :-----------: |
| Understanding a concept or subject         |           56% |
| Researching for assignments and projects   |           52% |
| Generating initial ideas/first drafts      |           46% |
| Writing/editing assignments and essays     |           41% |
| Helping to prepare for presentations       |           38% |
| Exam/quiz prep                             |           36% |
| Checking homework                          |           33% |
| Step-by-step homework help                 |           29% |

Anthropic's analysis found that most students use Claude for higher-order thinking skills such as creating (39.8%) and analyzing (30.2%), rather than lower-order skills like applying (10.9%) and understanding (10.0%). A survey of over 73,000 California State University students showed 64% agree that AI has positively affected their learning experience. 48% of institutions now have policies governing acceptable uses of generative AI, an increase of 9 percentage points since 2025.

---

## 7.3 K–12 CS and AI Education

### United States — Foundational Computer Science

Between the 2017–18 and 2023–24 academic years, the percentage of U.S. high schools offering CS increased from 35% to 60%. The national average has held steady since 2023–24, with 60% of high schools offering foundational CS classes in 2024–25. CS education access varies by school size (small: 44%, medium: 77%, large: 91%), geographic area (rural: 57%, urban: 59%, suburban: 71%), and Title I status (Title I: 60%, Non-Title I: 65%).

**Access to foundational CS courses by race/ethnicity, 2025** *(Fig. 7.3.7)*

| Group                        | % of students |
| ---------------------------- | :-----------: |
| Native American              |           70% |
| Black                        |           80% |
| Hispanic/Latino              |           80% |
| Native Hawaiian/Pacific Islander |         81% |
| White                        |           82% |
| Two or more races            |           84% |
| Asian                        |           91% |

**CS participation nationally:** 6.1% of U.S. students were enrolled in CS in 2024–25. Arkansas (25.1%) and South Carolina (25.7%) report the highest participation rates.

### Advanced Computer Science

AP exam participation in CS has grown steadily since 2016, reaching **254,800 exams in 2024** (up from 243,180 in 2023). However, AP exam growth slowed from 21% between 2022 and 2023 to just 5% between 2023 and 2024. Female students take the AP CS exam less often than male students. Asian students, multiracial boys, and white boys are overrepresented among AP CS exam takers.

### K-12 Student Use of AI Tools

Estimates on student use of AI to complete school-related tasks range from about 50% to 84%, based on survey data. High school students report using generative AI most often for conducting research and finding sources (51%), editing or revising essays (50%), and brainstorming ideas (50%). Only about half of middle and high schools have policies regarding AI use; only 28% permit AI use in some circumstances, while 22% do not. Only 36% of students described their school's policies as extremely clear, and only 6% of teachers say their schools had clear, comprehensive policies.

### Education Standards and Policies

As of January 2026, **30 states** have issued guidance on AI *in* education. Regarding policy focused specifically on AI education, 17 states have issued guidance that clarifies CS as foundational to AI, and five states have allocated specific professional development funding for AI education. **45 states** have adopted K–12 CS standards, while only 6 have significant AI-specific content. Revised CSTA K–12 Standards, slated for release in summer 2026, will delineate significant AI-related learning goals across grades K–12.

**States with significant AI content in K-12 CS standards** *(Table 4.1)*

| State      | Year         | Earliest grade | Approach                     |
| ---------- | ------------ | -------------- | ---------------------------- |
| Alabama    | 2025 (draft) | 1st grade      | Integrated across concepts   |
| Arkansas   | 2025         | 9–12 elective  | Elective course              |
| Colorado   | 2024         | 2nd grade      | Standalone concept           |
| Florida    | 2024         | 1st grade      | Integrated across concepts   |
| North Dakota | 2025       | Kindergarten   | Integrated across concepts   |
| Ohio       | 2022         | Kindergarten   | Standalone concept           |
| Virginia   | 2024         | 4th grade      | Integrated across concepts   |
| Wisconsin  | 2025 (draft) | Pre-K–5        | Integrated across concepts   |

**Federal K–12 AI Guidance:** An April 2025 Executive Order, *Advancing Artificial Intelligence Education for American Youth*, sought to define a national strategy for developing AI competency from K–12 through postsecondary education by promoting early student exposure to AI, integrating AI into instruction, and expanding professional learning for educators. It establishes a White House AI Education Task Force and directs the Departments of Education, Labor, NSF, and Agriculture to prioritize AI in grants, research, teacher preparation, apprenticeships, and workforce pathways.

> **HIGHLIGHT: The Implementation Gap in K–12 AI Policy**
>
> State-level AI guidance is largely nonbinding and decentralized. Most states rely on existing federal laws (COPPA, FERPA) rather than AI-specific mandates. The responsibility for local policy development, tool vetting, and implementation falls on local education agencies, meaning the rigor of AI education and pace of adoption are determined by local capacity. Teacher preparation is also an identified gap — there are currently no state-level standards for programs or funding. AP Computer Science, one of the most common pathways to advanced CS coursework in U.S. high schools, does not include AI-specific content. Policy guidance, teacher training, and curriculum would all need to align for AI education to reach students consistently, and at present, gaps remain in all three.

### Global AI and CS Education

Two countries made significant strides in implementing AI education in 2025:
- **China** — Beijing, Guangdong, and Hangzhou all began requiring AI education in the 2025–26 school year following release of the General AI Education Guide for Primary and Secondary Schools. Starting with elementary students learning AI literacy skills and ending with high school students designing AI systems.
- **UAE** — similarly mandated AI education for all grade levels starting in the 2025–26 school year, with a grade-level curriculum covering foundational concepts, data and algorithms, software use, innovation, and ethical awareness.

In 2025, approximately **93% of the world's countries taught CS**. 30% mandate CS education in primary or secondary school, while 63% have CS available in at least some schools but do not mandate it.

---

## 7.4 AI Skill Acquisition

Formal education is one entry point into AI, but as the technology reshapes jobs across sectors, upskilling and reskilling have become central to lifelong learning.

### AI Skill Penetration

LinkedIn's relative AI skill penetration rate measures how prominently AI skills feature in people's profiles compared with a global average. **India leads at 2.95**, followed by the United States at 2.02 and Germany at 1.83. However, these countries also show a persistent gender gap: in India, men list AI skills at more than 1.5 times the rate of women (3.1 vs. 1.9); in the United States, men at 2.1 vs. women at 1.4.

**Relative AI skill penetration rate, 2025** *(Fig. 7.4.1)*

| Country         | Rate |
| --------------- | ---- |
| 🇮🇳 India       | 2.95 |
| 🇺🇸 US          | 2.02 |
| 🇩🇪 Germany     | 1.83 |
| 🇬🇧 UK          | 1.55 |
| 🇨🇦 Canada      | 1.54 |
| 🇫🇷 France      | 1.53 |
| 🇧🇷 Brazil      | 1.48 |
| 🇪🇸 Spain       | 1.47 |
| 🇸🇬 Singapore   | 1.43 |
| 🇮🇱 Israel      | 1.38 |
| 🇦🇪 UAE         | 1.37 |

### AI Skills Diffusion

The AI Skills Diffusion Index tracks how much AI skills adoption has grown within a country relative to its own baseline. Both AI literacy and AI engineering skills show recent increases across many countries, but the pace differs — AI literacy skills show steeper growth. Countries such as the UAE, Chile, and South Africa show rapid growth in AI engineering skills. In the United States, the fastest growing AI literacy skills were AI prompting and Microsoft Copilot Studio, while the fastest growing engineering skills were AI agents, AI productivity, and AI strategy.

**Fastest growing AI skills in the United States, 2025** *(Fig. 7.4.4)*

| Rank | AI engineering skills                    | AI literacy skills          |
| ---- | ---------------------------------------- | --------------------------- |
| 1    | AI agents                                | AI prompting                |
| 2    | AI productivity                          | Microsoft Copilot Studio    |
| 3    | AI strategy                              | GitHub Copilot              |
| 4    | Amazon Bedrock                           | Prompt engineering          |
| 5    | Large language model operations (LLMOps) | Microsoft Copilot           |

---

# Chapter 8: Policy and Governance

> **Overview:** Around the world, AI policy is no longer just about regulation. Governments are also investing to build and maintain their own capacity across the infrastructure, data, talent, and models that make up the technology. The number of countries with formal AI strategies continued to grow, with particular momentum among lower-income economies. Legislative activity continued to grow at every level, though in the United States, federal policy shifted toward deregulation even as state legislatures passed a record number of AI-related bills. Globally, advanced model development and large-scale compute remain concentrated in a small number of countries, while more governments pursue sovereign AI strategies.

## Chapter 8 Highlights

1. **National AI strategies are expanding fastest among countries that had no formal AI policy five years ago.** In 2024, more than half of newly adopted strategies came from emerging economies and, as of 2025, additional countries across sub-Saharan Africa, Central Asia, and the Middle East have strategies in active development.
2. **AI sovereignty, the goal of gaining more agency over domestic AI capabilities, is emerging as a central principle of national AI policy, but the infrastructure underpinning it is unevenly distributed.** Between 2018 and 2025, Europe and Central Asia expanded state-backed AI supercomputing clusters from 3 to 44. South Asia, Latin America, and the Middle East and North Africa have only reached between 2, 3 and 8 each.
3. **Regions are taking different approaches to data sovereignty.** Through 2024, East Asia and the Pacific had adopted 77 data localization measures, followed by sub-Saharan Africa with 71 and Europe and Central Asia with 66. North America recorded only 3.
4. **AI-related witnesses in U.S. congressional hearings have grown twentyfold since 2017.** The number rose from 5 in 2017 to 102 in 2025. Industry's share nearly tripled from 13% to 37%, making it the largest witness group, while academia's share fell to 15%.
5. **U.S. public investment in AI remains modest compared to private-sector spending.** Between 2013 and 2024, the United States invested approximately $20.4 billion in AI-related contracts and grants, against $285.9 billion in U.S. private investment in 2025 alone.
6. **European AI public commitments reached approximately $3.7 billion in contracts over 2013–2024.** The United Kingdom accounted for $1.6 billion, followed by Germany with $505 million and France with $320 million.

---

## 8.1 Major Global AI Policy News in 2025

| Date       | Event                                                                  | Summary                                                                                                           |
| ---------- | ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| Jan 23     | **US EO 'Removing Barriers to American Leadership in AI'**             | Rescinds earlier AI directives; establishes new policy to enhance U.S. AI dominance and remove regulatory barriers |
| Feb 1      | **UK to Criminalize AI-Generated Child Sexual Abuse Imagery**          | First country to introduce laws against AI tools generating sexualized images of children                         |
| Feb 2      | **1st Measures of the EU AI Act Come Into Effect**                     | Bans high-risk uses (predictive policing, emotion recognition); sets stage for stricter rules                     |
| Feb 11     | **AI Action Summit: US and UK Refuse to Sign Inclusive AI Declaration**| U.S. and UK decline to endorse declaration signed by 60 countries on inclusive, ethical AI                       |
| Mar 14     | **China Finalizes Mandatory Labeling Rules for AI-Generated Content**  | Final rules requiring clear labeling of AI-generated and synthetic media                                          |
| Mar 24     | **Zimbabwe Partners With Nvidia to Launch Africa's 1st AI Factory**    | First dedicated AI factory on the continent                                                                       |
| Mar 25     | **Utah Enacts the Mental Health Chatbot Act (HB 452)**                 | Mandates disclosure of AI use, bans advertising, prohibits sharing personal data                                  |
| Apr 3      | **Kigali Summit Highlights Africa's AI Opportunity and Labor Risks**   | Inaugural Global AI Summit on Africa                                                                              |
| Apr 16     | **Montana Enacts the Right to Compute Act (SB 212)**                  | Pro-innovation legal framework protecting rights to own and use computational resources                           |
| May 17     | **Africa Declares AI a Strategic Priority**                            | African Union identifies AI as central strategic priority                                                         |
| May 19     | **US Enacts the Take It Down Act**                                     | Covers deepfake content; addresses distribution of nonconsensual intimate imagery                                 |
| Jun 17     | **California AI Policy Report Warns of 'Irreversible Harms'**          | Highlights biological and nuclear misuse risks; proposes safety, transparency, and whistleblower frameworks       |
| Jun 17     | **G7 Issues Joint Statement on AI Governance**                         | Joint declaration committing to coordination on AI safety, risk management, and standards                        |
| Jun 22     | **Texas Signs the Responsible AI Governance Act (TRAIGA)**             | Strict rules for high-impact AI; bans uses that discriminate against protected classes                            |
| Jul 1      | **US Senate Strikes 10-Year Federal Moratorium on State AI Regulation**| Allows states to proceed with their own AI oversight laws                                                         |
| Jul 10     | **EU Releases Voluntary Code of Practice for General-Purpose AI**      | Guides businesses complying with EU AI Act for general-purpose models                                            |
| Jul 23     | **US Launches 'America's AI Action Plan' and 3 Executive Orders**      | Broad AI strategy covering innovation, infrastructure, and diplomacy                                              |
| Jul 26     | **China Announces Action Plan for Global AI Governance**               | 13-point road map to advance global AI coordination and standards                                                |
| Jul 30     | **Creators' Organizations Condemn EU AI Act Implementation**           | 38 global creative-industry bodies criticize the EU AI Act as undermining cultural rights                        |
| Aug 2      | **EU's General-Purpose AI Obligations Take Effect**                    | Risk assessments, transparency disclosures, and mitigation measures for systems with systemic risk               |
| Aug 26     | **AI Companion Lawsuits Prompt Renewed Scrutiny**                      | Teen suicide linked to AI companion; lawmakers increase scrutiny of AI companion and child-safety safeguards      |
| Aug 26     | **UN Launches Global Scientific Panel on AI Governance**               | Independent International Scientific Panel on AI + Global Dialogue on AI Governance                              |
| Sep 17     | **Italy Becomes First EU Member State to Pass an AI Law**              | National AI legislation to complement EU-level regulation                                                        |
| Sep 29     | **California Enacts Landmark AI Safety/Transparency Law (SB 53)**     | Requires large AI-model developers to disclose safety protocols and protect whistleblowers                       |
| Oct 8      | **European Commission Launches European Strategy for AI in Science**   | Aims to reinforce Europe's technological and scientific leadership                                               |
| Oct 13     | **California Enacts New AI Laws**                                      | SB243 (companion bots), AB853 (provenance data), AB621 (deepfakes)                                              |
| Nov 5      | **UNESCO Adopts Global Standards on Neurotechnology**                  | International standards covering AI-driven neurotechnology and "neural data" rights                              |
| Nov 24     | **US Executive Order Launches the Genesis Mission**                    | Major national initiative to accelerate scientific discovery using AI; compared in ambition to Manhattan Project  |
| Dec 11     | **Launch of Pax Silica Initiative**                                    | U.S.-led cooperation on trusted technology and AI-relevant supply chains                                         |
| Dec 12     | **US Executive Order Seeks to Curb State-Level AI Laws**               | Aimed at limiting or preempting state AI regulation to reduce regulatory burden                                  |

---

## 8.2 National AI Strategies

More countries adopted national AI strategies in 2024 and 2025, especially within emerging economies. New frameworks have surfaced across sub-Saharan Africa (Ethiopia, Ghana, Nigeria), South and Central Asia (Sri Lanka, Nepal), and Latin America and the Caribbean (Costa Rica, Jamaica). High-income economies continue to contribute new strategies as well, though at a slower pace, with a focus on consolidating earlier frameworks. European countries such as Malta have released updated strategies to align with EU AI Act requirements.

The next challenge is implementation and strengthening regulatory capacity, particularly in Africa, where many countries still lack formal strategies and risk falling behind in AI governance and readiness.

---

## 8.3 AI Sovereignty

AI sovereignty describes a state's capacity to act deliberately and make independent decisions over the development, deployment, and governance of AI systems within its jurisdiction. The sovereignty debate has expanded beyond data and infrastructure to include compute, model development, talent, and responsible AI deployment.

### Infrastructure Sovereignty

Between 2018 and 2025, the number of state-backed AI supercomputing clusters grew dramatically:
- 🇨🇳 China: **85** clusters
- 🇪🇺 Europe and Central Asia: **44** clusters (grew from 3)
- 🇺🇸 North America: **41** clusters (grew ~7×)
- East Asia and Pacific: **27** clusters
- Latin America and Caribbean: **8** clusters
- South Asia: **2** clusters

OpenAI's Stargate project extends beyond the United States through country-level partnerships including the UAE, UK, Argentina, South Korea, India, and Norway. Nvidia's "AI Factory" model builds in-country compute capacity in partnership with domestic telecommunications providers.

### Data Sovereignty

Data localization measures have increased across nearly every region since 2000. Regional patterns as of 2024:
- 🟦 High-localization: East Asia & Pacific (77), Sub-Saharan Africa (71), Europe & Central Asia (66)
- 🟦 Moderate: Middle East & North Africa (44), Latin America & Caribbean (36), South Asia (24)
- ⬜ Low: North America (**3** measures — long-standing "flow-first" policy orientation)

### Model Sovereignty

Cumulative U.S. model releases grew from 237 to **1,618** between 2018 and 2025. China exhibits a similar acceleration, where model releases more than quintupled from 151 to **849** between 2022 and 2025. Europe and Central Asia show steady growth, increasing from 127 to **666** models (UK: 229, France: 141). East Asia and Pacific (excluding China) grew from 39 to 330 models. Open-source frameworks have lowered barriers to entry, and a growing number of regions are building and deploying their own models, including Chile's Latam-GPT, the UAE's Falcon series, and Singapore's SEA-LION.

### Application Sovereignty

A small set of countries — most prominently the United States, China, and several European economies (UK, Germany, France) — show high-intensity investment across nearly all application categories. Most other countries display concentrated areas of focus:
- Germany: industrial applications (particularly manufacturing)
- Estonia: education technologies
- Sub-Saharan African countries: financial applications (led by South Africa)
- Israel: security and defense applications

### Talent Sovereignty

A fifth dimension of AI sovereignty is a nation's ability to develop and retain the human capital needed to build, deploy, and govern AI systems. The country-level distribution and mobility patterns of AI authors and inventors offer a direct window into this dimension (also discussed in Sections 1.8 and 4.4 of the report).

---

Cross-border AI talent circulation has slowed recently. Both inflows and outflows are declining, suggesting that talent is increasingly staying within national or regional systems rather than circulating globally. The United States is currently the primary global attractor of top AI talent, though its lead is rapidly narrowing. India is transitioning from a net exporter to a net absorber of talent. The Middle East and North Africa are making incremental gains, with new talent hubs emerging with the support of targeted policy and investment.

---

## 8.4 AI and Policymaking

Legislative activity is a signal of how governments are responding to AI beyond national strategies. This section tracks AI-related bills passed across G20 countries.

### Global Legislative Records on AI

In 2016, there were no AI-related laws on record among G20 countries. Since then, legislative activity has been on the rise. Between 2016 and 2025, the United States passed the most AI-related bills with **25 in total**, followed by South Korea with 17. Japan, France, and Italy were also relatively active, each passing 9 to 10 laws. The total number of AI-related bills passed in G20 countries reached **150 in 2025**, up from fewer than 10 in 2020.

**AI-related bills passed by G20 countries, 2016–25 (sum)** *(Fig. 8.4.3)*

| Country       | Bills |
| ------------- | ----- |
| United States |    25 |
| South Korea   |    17 |
| France        |    10 |
| Japan         |    10 |
| Italy         |     9 |
| United Kingdom|     6 |
| Germany       |     5 |
| Russia        |     5 |
| Brazil        |     3 |
| Australia     |     2 |
| China         |     1 |
| India         |     1 |

> **HIGHLIGHT: A Closer Look at Global AI Legislation**
>
> Notable 2025 AI laws:
> - **US: Take It Down Act** — criminalizes nonconsensual distribution of intimate imagery including AI-generated deepfakes; mandates platforms remove reported content within 48 hours
> - **Italy: Law No. 132/2025** — establishes a national framework for responsible, transparent, human-centered AI; aligned with EU AI Act
> - **Japan: Act on the Promotion of Research and Development and Utilization of AI-Related Technology** — national framework promoting AI R&D; establishes AI Strategy Headquarters
> - **South Korea: Framework Act on the Development of AI and Creation of a Foundation for Trust** — overarching policy and governance framework for trustworthy and ethical AI

### US Legislative Records — State Level

Across all states, the total number of AI-related bills passed increased from fewer than 10 in 2020 to **150 in 2025**. California enacted 20 AI-related bills in 2025 — double the total in New York (10) and two-thirds more than Texas (12). Over the full 2016–2025 period, California had more than double any other state with 62 bills enacted. Maryland (28), Virginia (25), and Utah (24) also had records reflecting consistent activity across multiple legislative cycles. Missouri and Rhode Island have not enacted any AI-related legislation to date.

**AI-related bills passed by select US states, 2016–25** *(Fig. 8.4.5)*

| State       | Bills |
| ----------- | ----- |
| California  |    62 |
| Maryland    |    28 |
| Virginia    |    25 |
| Utah        |    24 |
| New York    |    18 |
| Texas       |    17 |
| Illinois    |    15 |

> **HIGHLIGHT: State AI Legislation Amid Shifting Federal Policy**
>
> While federal AI policy shifted toward deregulation in 2025, state legislatures continued to move ahead with AI-specific laws. State policies are developing across different tracks, including targeted protections against discrimination, misinformation, and abuse. Key state laws include Utah's Mental Health Chatbot Act (HB 452), Montana's Right to Compute Act (SB 212), and Texas' Responsible AI Governance Act (TRAIGA). Colorado's Artificial Intelligence Act (May 2024) was among the first state laws targeting algorithmic discrimination in hiring, housing, and medical care. Watermarking and provenance requirements have gained traction in Washington (HB 1170), Illinois (SB 1929), and Florida (HB 369). In December 2025, a White House executive order directed the DOJ to establish an AI Litigation Task Force to challenge state AI laws considered overly burdensome, and tied some federal funding to states' willingness to avoid conflicting AI legislation. The future of U.S. state-level regulation on AI remains uncertain.

### US Congressional Hearings

Congressional attention to AI has increased by almost **twentyfold since 2017**. Witness counts rose from 18 in 2022 to 131 in 2023 and remained high at 102 in 2025. The composition of those witnesses has shifted over time:

| Congress | Academia | Industry | Government | Other |
| -------- | -------- | -------- | ---------- | ----- |
| 115th    |      26% |      13% |        35% |   26% |
| 116th    |      35% |      19% |        17% |   30% |
| 117th    |      19% |      26% |        25% |   28% |
| 118th    |      25% |      27% |        15% |   33% |
| 119th    |      15% |      37% |        10% |   38% |

General AI governance (113 witnesses total) and national security and defense (74) drew the most witnesses. The House has been more active than the Senate in most subject areas.

### US Regulations

Federal regulatory activity on AI has grown from one recorded action in 2016 to **58 in 2025**. The sharpest increase came after 2022. The direction of federal AI policy changed in early 2025 when the Trump administration revoked Biden's Executive Order 14110 on safe, secure, and trustworthy AI development and issued a new executive order reorienting federal policy toward reducing regulatory constraints and promoting innovation.

**Most active agencies in AI regulations, 2025** *(Fig. 8.4.11)*

| Agency                              | 2025 actions |
| ----------------------------------- | :----------: |
| Executive Office of the President   |           28 |
| Commerce/Industry & Security Bureau |            8 |
| HHS                                 |            5 |
| Commerce Department                 |            8 |
| Education Department                |            4 |
| Centers for Medicare/Medicaid       |            5 |

> **HIGHLIGHT: Key US Federal AI Regulations in 2025**
>
> - **Framework for AI Diffusion** (Commerce/ISB) — Updated U.S. export controls for advanced computing and AI model weights; rescinded by Trump administration in May 2025
> - **Preventing Access to U.S. Sensitive Personal Data** (DOJ) — Restricts certain data transactions involving countries of concern or covered persons, focusing on bulk sensitive personal data
> - **Advancing U.S. AI Infrastructure** (Exec. Office) — Directs federal government to fast-track buildout of domestic AI infrastructure on federal lands
> - **Removing Barriers to American Leadership in AI** (Exec. Office) — Sets foundational U.S. policy of maintaining global AI dominance by dismantling regulations seen as obstacles
> - **Ensuring a National Policy Framework for AI** (Exec. Office) — Creates AI Litigation Task Force; directs federal agencies to evaluate problematic state AI laws
> - **Advancing AI Education for American Youth** (Exec. Office) — Directs federal government to expand early exposure to AI concepts; establishes White House Task Force on AI Education
> - **Promoting the Export of the American AI Technology Stack** (Exec. Office) — Coordinates federal initiative to promote export of "full-stack" American AI technology packages

---

## 8.5 Public Investment in AI

Public spending on AI reflects how governments are translating national strategies and policy commitments into resource allocation.

### United States

Between 2013 and 2024, the United States invested approximately **$20.5 billion** toward AI-related activities: $15.9 billion in grants, $3.9 billion in contracts, and $650 million in Other Transaction Agreements (OTAs). Since 2020, AI-related grant spending has accelerated compared to contracts and OTAs. In 2024, grants accounted for $5.1 billion, 32% of their cumulative total since 2013.

**US AI-related public spending summary** *(Fig. 8.5.3)*

| Statistic                           | Grants   | Contracts | OTAs   |
| ----------------------------------- | -------- | --------- | ------ |
| Number of awards                    | 22,364   | 3,347     | 185    |
| Total ($B)                          | 15.9     | 3.9       | 0.7    |
| Median award ($K)                   | 304      | 149.9     | 999.7  |
| Average award ($K)                  | 710.1    | 1,167.9   | 3,528.8 |

Geographically, U.S. public AI investment through contracts and OTAs is highly concentrated: Virginia received $1.09 billion, California $0.67 billion, and Maryland $0.55 billion — together nearly 60% of total contract and OTA spending. AI-related grants have been more broadly dispersed: California ($2.37 billion), Massachusetts ($1.3 billion), and New York ($1.15 billion) received the largest allocations.

**Spending Across Agencies:** The Department of Defense led in AI-related contract and OTA spending, accounting for 74.1% of the $810 million total in 2024 and 73% of the total spend ($4.6 billion) across the whole period. AI-related grants have been channeled mainly through HHS (including the NIH) and the National Science Foundation — each accounting for roughly 40% of the total by 2024.

### Europe

European nations collectively committed approximately **$3.7 billion** in contracts over the 2013–24 period. The United Kingdom accounted for the largest share with $1.6 billion (738 contracts), followed by Germany ($505 million, 611 contracts) and France ($320 million, 161 contracts). In 2024 alone, the U.K. committed $454.4 million (28% of its decade total) and Germany committed $206.6 million (40% of its total). Despite their higher spending and contract volumes, the UK, Germany, and France have a median contract value below $500,000 — far lower than smaller European countries such as Denmark (almost $1.1 million). In Europe, government bodies accounted for the largest share of AI-related contract spending at 62.6%, followed by health (13.9%) and education (13.7%).

---

---

# Chapter 9: Public Opinion

**Overview:** Public views of AI are now shaped by a central tension, as optimism about the technology's benefits often coexists with anxiety about its broader effects. Majorities in most countries say AI's benefits outweigh its drawbacks, but nervousness is growing and trust in institutions to manage the technology remains uneven. AI experts and the general public view the technology's trajectory very differently, with wide gaps on employment, the economy, and healthcare. Southeast Asian countries are consistently the most optimistic and most trusting of their own governments to regulate AI, while North America and Europe report lower expectations and greater skepticism. This chapter tracks these patterns across 30-plus countries, drawing on several large-scale surveys conducted between 2024 and 2026 from Ipsos AI Monitor, Pew Research Center, the University of Melbourne/KPMG Global AI Survey, CHIP50 survey, the LEAP Survey and Elon University's Human Capacities survey.

## Chapter Highlights

1. **AI optimism is rising, but so is anxiety.** Globally, the share of respondents who say AI products and services offer more benefits than drawbacks rose from 55% in 2024 to 59% in 2025, even as the share saying these products make them nervous increased to 52%.

2. **Southeast Asian countries remain among the world's most optimistic about AI.** In Malaysia, Thailand, Indonesia, and Singapore, more than 80% of respondents say AI will profoundly change their lives in the next 3–5 years, with Malaysia posting the largest increase from 2024.

3. **India saw the sharpest rise in AI nervousness of any country surveyed.** Between 2024 and 2025, India registered the sharpest rise in concern around AI usage (+14 percentage points) with only a modest increase in excitement (+2).

4. **Workplace AI usage is higher in several emerging economies than in many advanced ones.** In 2025, 58% of employees globally reported using AI at work on a semiregular or regular basis, but in India, China, Nigeria, the United Arab Emirates, and Saudi Arabia, the share exceeded 80%.

5. **AI experts and the U.S. public have very different perspectives on AI's future, except on elections and personal relationships.** On how people do their jobs, 73% of experts expect a positive impact, compared to just 23% of the public, a 50-point gap. Similar divides appear for the economy (69% vs. 21%) and medical care (84% vs. 44%).

6. **Nearly two-thirds of Americans (64%) expect AI to lead to fewer jobs over the next 20 years, while only 5% expect more.** Experts were less pessimistic (39% fewer, 19% more) but forecast far faster adoption, expecting generative AI to assist 18% of U.S. work hours by 2030 versus the public's estimate of 10%.

7. **AI companionship is still niche, and global views vary widely.** More than half of respondents worldwide (52%) reported some excitement about using AI for companionship, compared with just 42% in the United States. Experts forecast that 10% of U.S. adults will use an AI companion daily by 2027, rising to 30% by 2040.

8. **The United States reported the lowest trust in its own government to regulate AI responsibly of any country surveyed, at 31%.** The global average was 54%, with Southeast Asian countries leading (Singapore 81%, Indonesia 76%).

9. **Across all 50 U.S. states, concern about too little AI regulation outweighs concern about too much.** Nationally, 41% of respondents said federal AI regulation will not go far enough, compared with 27% who said it will go too far, though more than one-third were unsure.

10. **Globally, the EU is trusted more than the United States or China to regulate AI effectively.** Across 25 countries in Pew's 2025 survey, a median of 53% said they trust the EU, compared to 37% for the United States and 27% for China.

---

## 9.1 Global Sentiment Toward AI

Since 2022, Ipsos has conducted its annual AI Monitor survey to track public attitudes and perceptions of artificial intelligence worldwide. The 2025 survey was conducted from March 21 to April 4 and covered 30 countries, with a sample size of 23,216 adults.

There are some modest shifts over the years in respondents' opinions, though self-reported AI literacy remains consistent. Over half of all respondents reported having a good understanding of what AI is and which products and services to use. Over the last year, nervousness has also increased, with the proportion of people who say AI products make them nervous rising by 2 percentage points, to 52%. In tandem, more respondents expressed optimism that the benefits outweigh the drawbacks of AI-enabled products and services, up to 59% from 55% in 2024.

**Global opinions on products and services using AI (% of total), 2022–25** *(Fig. 9.1.1)*

| Statement                                                                 | 2025 | 2024 | 2023 | 2022 |
| ------------------------------------------------------------------------- | ---- | ---- | ---- | ---- |
| I have a good understanding of what AI is                                 | 68%  | 67%  | 64%  | —    |
| I know which types of products and services use AI                        | 53%  | 51%  | 50%  | —    |
| AI products and services have profoundly changed my daily life (past 3–5y)| 53%  | 49%  | 49%  | —    |
| AI products and services will profoundly change my daily life (next 3–5y) | 67%  | 66%  | 60%  | —    |
| AI products and services have more benefits than drawbacks                | 59%  | 54%  | 52%  | —    |
| I trust AI to not discriminate or show bias toward any group of people    | 55%  | 56%  | —    | —    |
| I trust companies using AI will protect my personal data                  | 48%  | 50%  | —    | —    |
| AI products make me excited                                               | 53%  | 54%  | —    | —    |
| AI products make me nervous                                               | 52%  | 52%  | 39%  | —    |
| AI products and services should have to disclose that use                 | 79%  | —    | —    | —    |

The increase in optimism is not uniform across all surveyed countries. Several European countries report higher levels of optimism over 2022–2025, including Germany (+12 percentage points), France (+10), China (+9), and Great Britain (+5), though their overall sentiment remained lower than in parts of Asia and Latin America.

Southeast Asian nations are among the most optimistic about the future of AI. In Malaysia, Thailand, Indonesia, and Singapore, over 80% of respondents expect AI to profoundly change their lives over the next three to five years. Malaysia showed the largest increase (+9) from 2024. Respondents from these countries also report higher levels of excitement than nervousness about AI-enabled products and services.

> **HIGHLIGHT:** When looking at year-over-year percentage point changes, global nervousness has increased (+3) and excitement declined (−1) relative to 2024. India shows the sharpest increase in concern around AI usage (+14) with only a modest increase in excitement (+2).

Across countries, excitement and nervousness about AI do not align closely. North American and European countries are generally clustered at lower levels of excitement and higher levels of nervousness. China and Indonesia show the highest levels of excitement, with nervousness below 50%.

Despite increased nervousness, many respondents continue to associate AI with practical personal benefits, particularly time savings and entertainment. Globally, 56% of respondents believed AI would reduce the amount of time it takes them to get things done; this figure was even higher in China (78%) and in Southeast Asian countries (>60%). However, respondents were less sure about AI's potential to positively impact their country's economy or job market. North American and European respondents were more skeptical that AI would make their jobs better. In the United States, 33% of respondents said AI would make their jobs better, as opposed to them worse or having no impact, compared to the global average of 40%.

### Global Perceptions of AI's Impact on Jobs

In both 2024 and 2025, Ipsos asked respondents how likely they thought it was that AI would change their job or completely replace it within the next five years. In 2025, 22% of respondents said it was "very likely" AI would change how they do their current job, compared to 21% in 2024. In both years, the share that said it was "not likely" remained unchanged at 32%. Expectations around job replacement showed the same consistency. In 2024 and 2025, 11% of respondents reported that it was "very likely" AI would replace their job within the next five years, and 56% said this was "not likely."

When asked whether AI is generally more likely to create new jobs or eliminate existing ones, views in 2025 were divided. Nigeria, Japan, Mexico, the United Arab Emirates, South Korea, and India all expected AI to create more jobs than it eliminates, with shares above 60%. The United States and Canada sat at the opposite end, where 67% and 68% of respondents expected AI to eliminate jobs and disrupt industries.

**Global expectations about AI creating new jobs vs. eliminating jobs, 2025** *(Fig. 9.1.8)*

| Country         | Create new jobs | Eliminate jobs |
| --------------- | --------------- | -------------- |
| 🌐 Global       | 50%             | 50%            |
| 🇳🇬 Nigeria     | 73%             | 27%            |
| 🇯🇵 Japan       | 69%             | 31%            |
| 🇲🇽 Mexico      | 64%             | 35%            |
| 🇦🇪 UAE         | 63%             | 37%            |
| 🇰🇷 South Korea | 63%             | 37%            |
| 🇮🇳 India       | 63%             | 37%            |
| 🇧🇷 Brazil      | 59%             | 41%            |
| 🇸🇬 Singapore   | 57%             | 43%            |
| 🇦🇷 Argentina   | 50%             | 50%            |
| 🇿🇦 South Africa| 45%             | 55%            |
| 🇵🇱 Poland      | 45%             | 55%            |
| 🇪🇸 Spain       | 45%             | 55%            |
| 🇦🇺 Australia   | 43%             | 57%            |
| 🇫🇷 France      | 43%             | 57%            |
| 🇧🇪 Belgium     | 43%             | 57%            |
| 🇮🇪 Ireland     | 41%             | 59%            |
| 🇮🇹 Italy       | 41%             | 59%            |
| 🇩🇪 Germany     | 39%             | 61%            |
| 🇬🇧 UK          | 38%             | 62%            |
| 🇨🇦 Canada      | 32%             | 68%            |
| 🇺🇸 US          | 29%             | 71%            |

Respondents were also asked whether AI would make the job market and their own jobs better, worse, or stay the same over the next five years. Optimism on both measures is low, under or around 50%, in most countries surveyed. China, Indonesia, Thailand, and Singapore report more positive expectations around AI's impact on jobs, both individually and economy-wide. North America and Europe have lower expectations, though respondents there were more positive about how AI might improve their individual jobs compared to the overall job market.

> **HIGHLIGHT: Global AI Use in the Workplace**
>
> Since 2022, the use of AI technology within organizations has become more prevalent. The University of Melbourne fielded a global survey of 48,340 people across 47 countries, examining how employees are adopting and using AI at work.
>
> Globally, the share of employees who intentionally use AI at work continues to grow. In 2025, 58% of employees reported using AI on a semiregular or regular basis, and just over half (53%) said they trust AI for work purposes. Employees in emerging economies remain the most active users of AI in the workplace: in India, China, Nigeria, the United Arab Emirates, and Saudi Arabia, over 80% of respondents said they regularly use AI at work, and trust levels in these countries are similarly high. By contrast, in most North American and European countries, about half of employees report using AI tools regularly, while trust tends to fall several points lower, between 40% and 48%.
>
> Consistent with usage and trust levels, organizational support was reported highest in emerging economies. In India, around 85%–90% of respondents said their organization supports AI strategy, literacy, and governance. Nigeria, Egypt, China, and the UAE also rank among the top countries for organizational support. At the other end, respondents in Japan, Korea, and Portugal report the lowest levels of support for AI literacy, along with less confidence in responsible AI governance.
>
> Overall, most countries reported less organizational support for responsible AI governance, in comparison to literacy and strategy.

---

## 9.2 US Public and Expert Views on AI's Societal Impact

This section draws on multiple U.S.-focused surveys to compare how the public and AI experts view AI's societal impact. The main sources are Pew Research Center's 2024 survey of U.S. adults and AI experts, Elon University's Imagining the Digital Future Center's 2025 survey on expected effects on human capacities by 2035, and the Longitudinal Expert AI Panel (LEAP), conducted by the Forecasting Research Institute. For the Pew survey, AI experts were U.S.-based authors or presenters at AI-related conferences in 2023 or 2024 who reported that their work or research relates to AI.

Across nearly every topic surveyed, experts report more optimism than the U.S. public. The largest gaps show up around the future of work: 73% of AI experts said AI will have a positive impact on how people do their jobs, compared to 23% of U.S. adults. Similar gaps appear for the economy (69% vs. 21%), K–12 education (61% vs. 24%), and medical care (84% vs. 44%). For both groups, however, optimism is low in domains tied to trust and social connection, including elections, news, and personal relationships.

**US perceptions of AI's societal impact: general public vs. experts** *(Fig. 9.2.1)*

| Domain                    | 🧑 U.S. adults | 🔬 AI experts |
| ------------------------- | ------------- | ------------ |
| Medical care              | 44%           | 84%          |
| K–12 education            | 24%           | 61%          |
| How people do their jobs  | 23%           | 73%          |
| The economy               | 21%           | 69%          |
| Arts and entertainment    | 20%           | 48%          |
| The environment           | 20%           | 36%          |
| The criminal justice system| 19%          | 32%          |
| The news people get       | 10%           | 18%          |
| Elections                 | 9%            | 11%          |
| Personal relationships    | 7%            | 22%          |

*% saying AI will have positive impact over next 20 years*

When asked to look ahead to 2035, the U.S. public is again more pessimistic than AI experts about the impact the technology is likely to have on key human traits such as thinking, learning, and creativity. U.S. adults are more likely than AI experts to anticipate negative effects on metacognition (53% vs. 36%), defined as the ability to think analytically about one's own thinking process, and decision-making (48% vs. 30%). For social and emotional intelligence, 51% of U.S. adults and 34% of experts expect AI to have a negative impact. Concern about mental well-being is high for both groups, with 55% of adults and 53% of experts saying AI will have a negative effect.

Beyond general sentiment, recent forecasting data shows even wider gaps in expected timelines and scale. The Longitudinal Expert AI Panel (LEAP) surveyed AI experts and the general public on specific AI milestones and adoption rates. Across 68 forecasts, experts consistently predicted much faster AI progress than the public.

In capability-focused forecasts, public views align with experts in only 9% of cases. When they diverge, the public expects slower progress 71% of the time. By 2030, AI experts expect higher accuracy on complex math problems (+25 points), more AI-assisted work (+8.2), and greater adoption of autonomous ride-hailing (+8). Looking further out to 2040, experts project a high likelihood of a transformative technological event occurring (+30) and much higher rates of daily AI companion use (+10) and AI-discovered drugs (+10).

Views on employment over the long term show a similar pattern. Nearly two-thirds or 64% of U.S. adults said AI will lead to fewer jobs in the next 20 years, while 5% said more jobs. Among experts, 39% predicted fewer jobs and 19% predicted more.

**Views on whether AI will create or eliminate jobs: general public vs. experts** *(Fig. 9.2.4)*

| Group      | Fewer jobs | Not much difference | More jobs | Not sure |
| ---------- | ---------- | ------------------- | --------- | -------- |
| U.S. adults| 64%        | 14%                 | 5%        | 16%      |
| AI experts | 39%        | 33%                 | 19%       | 10%      |

Experts forecast much faster workplace adoption than the public. The median prediction among experts is that generative AI will assist 8% of U.S. work hours in 2027, rising to 18% in 2030. In contrast, the public expects slower adoption, at 10% by 2030.

When asked about specific occupations, the U.S. public and AI experts identified certain jobs to be at higher risk for automation than others. There is strong consensus between the public and experts regarding automation risks for cashiers, journalists, and software engineers. AI experts see a greater risk for truck drivers and lawyers, while the U.S. public believes AI will lead to fewer jobs for teachers and medical doctors.

**Views on AI-driven job loss by occupation: general public vs. experts** *(Fig. 9.2.6)*

| Occupation              | 🧑 U.S. adults | 🔬 AI experts |
| ----------------------- | ------------- | ------------ |
| Cashiers                | 73%           | 73%          |
| Factory workers         | 67%           | 60%          |
| Journalists             | 59%           | 60%          |
| Software engineers      | 48%           | 50%          |
| Musicians               | 45%           | 35%          |
| Teachers                | 43%           | 31%          |
| Truck drivers           | 33%           | 62%          |
| Mental health therapists| 29%           | 27%          |
| Medical doctors         | 28%           | 18%          |
| Lawyers                 | 23%           | 38%          |

The gap in expert vs. public sentiment coincides with increasing awareness and adoption of AI in the United States. In 2025, 47% of U.S. adults said they had heard "a lot" about AI, up from 26% in 2022. Growth in awareness is steepest among younger adults, ages 18–29 (+29 percentage points since 2022), though it is also rising among those ages 65 and older (+13pp).

Adoption and frequency of use are also increasing. More than 60% of U.S. adults reported interacting with AI at least several times a week, and 31% said they interact with AI almost constantly or several times a day. Daily AI interaction is higher among younger adults, college-educated groups, Asian Americans, and men.

> **HIGHLIGHT: Views on AI Companions**
>
> AI companionship, defined as relationships with AI systems designed for ongoing emotional and social support, represents one of the more contentious emerging uses of AI technology. Experts predict that 10% of U.S. adults will use AI for companionship at least once a day by 2027, with that number rising to 15% by 2030 and 30% in 2040. The top quartile among experts' predictions forecast that more than 40% of the public will engage in daily AI companionship, while the top 10% predict over 60%. Expectations from the general public are significantly lower, at 20% by 2040.
>
> A 2026 Ipsos/Google survey found that 52% of worldwide respondents reported some level of excitement about using AI for companionship. In countries such as Nigeria, India, and the United Arab Emirates, over 20% of respondents said they were "extremely excited." The United States and Canada had the largest shares of respondents who were not excited at all, at 36% and 34%. Japan recorded very few "extremely excited" respondents, and had the highest share of "don't know" responses at 18%, nearly double the global average.
>
> AI companions differ from traditional task-oriented AI by prioritizing relationship building over functionality. Modern systems incorporate memory of past interactions, can recognize emotion, and adapt their responses to individual users' needs. Platforms like Replika, Character.ai, and XiaoICE have attracted user bases in the millions. Many users have reported forming emotional attachments to their AI companions, viewing them as friends, mentors, or romantic partners.
>
> The technology has both benefits and risks. Research shows that AI companions can reduce loneliness to a similar degree as interacting with another human, with users citing always-available support (11.9%) and a safe space for emotional expression (9.9%) as primary advantages. Mental health improvements were reported by 6.2% of users, and some credited their AI companions for helping them through crises. However, concerning patterns have emerged. Users frequently perceive chatbots as entities with needs, which poses a problem given the established correlation between emotional dependence and psychological distress. Critical questions remain about whether these relationships reduce loneliness sustainably or undermine existing relationships and increase social isolation.

---

## 9.3 Perceptions on AI Trust, Transparency, and Regulation

### Global Trust in Institutions

As AI becomes more embedded in daily life, the mechanisms around trust, transparency, and regulation also become more visible. In Ipsos' 2025 AI Monitor Survey, 79% of respondents said companies using AI should be required to disclose that usage. That view was shared across all 30 countries surveyed, even though overall trust in institutions was lower. Over half of respondents, or 54%, said they trust their government to regulate AI responsibly. The United States reported the lowest trust on this measure (31%). In parallel with the higher levels of optimism and excitement mentioned earlier, Southeast Asian countries reported the highest levels of trust in their governments, including Singapore (81%), Indonesia (76%), Malaysia (73%), and Thailand (70%).

**Trust in government regulation of AI by country (% of total), 2025** *(Fig. 9.3.1)*

| Country          | Trust (%) |
| ---------------- | --------- |
| 🇸🇬 Singapore    | 81%       |
| 🇮🇩 Indonesia    | 76%       |
| 🇲🇾 Malaysia     | 73%       |
| 🇹🇭 Thailand     | 70%       |
| 🇨🇱 Chile        | 67%       |
| 🇲🇽 Mexico       | 67%       |
| 🇨🇴 Colombia     | 66%       |
| 🇮🇳 India        | 65%       |
| 🇦🇷 Argentina    | 61%       |
| 🇵🇱 Poland       | 61%       |
| 🇵🇪 Peru         | 61%       |
| 🇨🇭 Switzerland  | 55%       |
| 🇪🇸 Spain        | 55%       |
| 🇿🇦 South Africa | 55%       |
| 🌐 Global        | 54%       |
| 🇮🇹 Italy        | 50%       |
| 🇮🇪 Ireland      | 49%       |
| 🇩🇪 Germany      | 49%       |
| 🇧🇪 Belgium      | 49%       |
| 🇹🇷 Turkey       | 48%       |
| 🇳🇱 Netherlands  | 48%       |
| 🇧🇷 Brazil       | 48%       |
| 🇰🇷 South Korea  | 46%       |
| 🇦🇺 Australia    | 46%       |
| 🇸🇪 Sweden       | 46%       |
| 🇫🇷 France       | 42%       |
| 🇨🇦 Canada       | 40%       |
| 🇬🇧 Great Britain| 39%       |
| 🇭🇺 Hungary      | 33%       |
| 🇯🇵 Japan        | 32%       |
| 🇺🇸 United States| 31%       |

A separate Pew global survey found that respondents tend to trust their own country most to regulate AI effectively, but trust in outside governments was mixed. Across the 25 countries surveyed, a median of 53% said they trust the EU to regulate AI effectively, compared to 37% for the United States and 27% for China. Trust in the Chinese government consistently received the lowest ratings across countries, while trust in the EU varied depending on whether respondents lived within or outside the EU.

However, even within the EU, trust levels were not uniform. Respondents in Germany and the Netherlands were among the most trusting of the EU's ability to regulate AI effectively, while Greece and Italy were among the least trusting. In the United States, views were evenly divided between trust (44%) and distrust (47%) in the government's ability to regulate AI effectively, and 43% said they trust the EU on AI regulation.

A separate Ipsos/Google survey shows a related divide in relation to public priorities. Globally, 58% of respondents said it was more important to foster advances in science, medicine, and other fields through AI innovation, compared to 41% who prioritized protecting industries that may be affected by AI through regulation. Most countries in the survey lean toward innovation, though South Africa, India, and Ireland were among the few where respondents were more likely to prioritize regulation.

**Global priorities: AI innovation vs. AI regulation, 2025** *(Fig. 9.3.3)*

| Country               | Advance science/medicine | Protect through regulation |
| --------------------- | ------------------------ | -------------------------- |
| 🌐 Global             | 58%                      | 41%                        |
| 🇳🇬 Nigeria           | 74%                      | 22%                        |
| 🇰🇷 South Korea       | 70%                      | 27%                        |
| 🇦🇷 Argentina         | 66%                      | 31%                        |
| 🇵🇱 Poland            | 67%                      | 32%                        |
| 🇯🇵 Japan             | 67%                      | 33%                        |
| 🇲🇽 Mexico            | 66%                      | 34%                        |
| 🇫🇷 France            | 65%                      | 35%                        |
| 🇩🇪 Germany           | 63%                      | 37%                        |
| 🇧🇪 Belgium           | 62%                      | 38%                        |
| 🇧🇷 Brazil            | 62%                      | 38%                        |
| 🇪🇸 Spain             | 54%                      | 46%                        |
| 🇬🇧 United Kingdom    | 54%                      | 46%                        |
| 🇦🇪 UAE               | 53%                      | 47%                        |
| 🇺🇸 United States     | 53%                      | 44%                        |
| 🇸🇬 Singapore         | 53%                      | 47%                        |
| 🇨🇦 Canada            | 52%                      | 48%                        |
| 🇮🇹 Italy             | 52%                      | 48%                        |
| 🇦🇺 Australia         | 52%                      | 48%                        |
| 🇮🇪 Ireland           | 48%                      | 52%                        |
| 🇿🇦 South Africa      | 47%                      | 53%                        |
| 🇮🇳 India             | 46%                      | 54%                        |

### US Attitudes Toward AI Regulation

In the United States, attitudes toward AI regulation vary meaningfully by geography. In 2025, the Civic Health and Institutions Project fielded a survey across 50 states, and asked respondents whether federal regulation of AI would go too far, not far enough, or "not sure." Across every state, concern about too little regulation outnumbers concern about too much regulation (41% vs. 27%), but the level of uncertainty is substantial, with more than one-third of respondents selecting "not sure."

New York and Tennessee reported the highest levels of concern that regulation will go too far (31%), while Missouri and Washington had the highest shares who said the government will not go far enough (48%). Across nearly every state, more respondents said regulation does not go far enough than said it goes too far. Roughly one in three respondents in most states said they were not sure, making uncertainty the second-largest category.

**Support for AI federal regulation by US state, 2025** *(Fig. 9.3.5)*

| State          | Go too far | Not far enough | Not sure |
| -------------- | ---------- | -------------- | -------- |
| Alabama        | 28%        | 36%            | 35%      |
| Arizona        | 25%        | 41%            | 33%      |
| Arkansas       | 28%        | 39%            | 33%      |
| California     | 28%        | 39%            | 33%      |
| Colorado       | 29%        | 45%            | 29%      |
| Connecticut    | 25%        | 42%            | 33%      |
| Florida        | 28%        | 40%            | 33%      |
| Georgia        | 29%        | 35%            | 35%      |
| Illinois       | 25%        | 41%            | 34%      |
| Indiana        | 26%        | 41%            | 34%      |
| Iowa           | 22%        | 43%            | 35%      |
| Kansas         | 25%        | 40%            | 35%      |
| Kentucky       | 24%        | 41%            | 35%      |
| Louisiana      | 26%        | 38%            | 36%      |
| Maryland       | 28%        | 41%            | 31%      |
| Massachusetts  | 24%        | 44%            | 32%      |
| Michigan       | 29%        | 43%            | 28%      |
| Minnesota      | 25%        | 47%            | 28%      |
| Mississippi    | 28%        | 34%            | 38%      |
| Missouri       | 20%        | 48%            | 32%      |
| Nebraska       | 25%        | 37%            | 38%      |
| Nevada         | 27%        | 46%            | 37%      |
| New Hampshire  | 24%        | 42%            | 34%      |
| New Jersey     | 27%        | 44%            | 30%      |
| New Mexico     | 24%        | 46%            | 29%      |
| New York       | 31%        | 36%            | 33%      |
| North Carolina | 28%        | 41%            | 31%      |
| Ohio           | 25%        | 41%            | 35%      |
| Oklahoma       | 23%        | 40%            | 37%      |
| Oregon         | 21%        | 45%            | 34%      |
| Pennsylvania   | 27%        | 45%            | 27%      |
| South Carolina | 25%        | 40%            | 35%      |
| Tennessee      | 31%        | 37%            | 32%      |
| Texas          | 29%        | 37%            | 34%      |
| Utah           | 25%        | 44%            | 31%      |
| Virginia       | 26%        | 42%            | 32%      |
| Washington     | 24%        | 48%            | 28%      |
| West Virginia  | 26%        | 40%            | 34%      |
| Wisconsin      | 25%        | 45%            | 30%      |

Across U.S. demographic groups, the strongest concern about insufficient AI regulation was reported among older adults, especially those 65 and older (51%). Education was associated with stronger support for more regulation, with 46% of college graduates saying the government will not go far enough, compared with 34% among respondents with a high school degree or less. Political affiliation was not a significant differentiator, although Democrats were more likely than Republicans to say regulation will not go far enough (45% vs. 40%), while concern about going too far is similar across parties (>25%).

**Attitude toward AI federal regulation in the US by demographic group, 2025** *(Fig. 9.3.6)*

| Group              | Go too far | Not far enough | Not sure |
| ------------------ | ---------- | -------------- | -------- |
| Total              | 27%        | 41%            | 33%      |
| Male               | 28%        | 41%            | 31%      |
| Female             | 25%        | 40%            | 35%      |
| White              | 25%        | 44%            | 32%      |
| Black              | 34%        | 32%            | 34%      |
| Hispanic           | 32%        | 35%            | 32%      |
| Asian              | 24%        | 37%            | 39%      |
| Ages 18–29         | 35%        | 38%            | 27%      |
| Ages 30–49         | 31%        | 36%            | 33%      |
| Ages 50–64         | 22%        | 42%            | 36%      |
| Ages 65+           | 16%        | 51%            | 33%      |
| Income <$30K       | 29%        | 35%            | 37%      |
| Income $30K–$69K   | 25%        | 43%            | 32%      |
| Income $70K–$99K   | 24%        | 44%            | 32%      |
| Income $100K+      | 29%        | 44%            | 26%      |
| HS or less         | 27%        | 34%            | 38%      |
| Some college       | 27%        | 42%            | 31%      |
| College+           | 26%        | 46%            | 28%      |
| Urban              | 29%        | 38%            | 33%      |
| Suburban           | 25%        | 42%            | 32%      |
| Rural              | 25%        | 39%            | 36%      |
| Republican         | 28%        | 40%            | 33%      |
| Independent/Other  | 25%        | 37%            | 37%      |
| Democrat           | 27%        | 46%            | 28%      |

---

# Appendix

## Chapter 1: Research and Development

### Environmental Impact Analysis

The AI Index estimated the carbon emissions of training language and vision models using a calculator proposed by Lacoste et al. (2019). The analysis focused on the training stage emissions—excluding embodied hardware production, idle infrastructure, and deployment emissions. The study examined four model categories: industry language models, academic language models, industry vision models, and academic vision models.

The calculator's accuracy was verified against published emission values. Calculator inputs included hardware type, GPU hours, provider, and compute region. For newer hardware like the H100 GPU (released in 2022), the A100 SXM4 80GB was used as a substitute in calculations. GPU hours were calculated by multiplying hardware quantity with training duration; these values were taken from Epoch AI's Data on AI models or from the technical paper for the model. Provider selection was based on known partnerships (e.g., Google models using GCP, OpenAI using Azure), while compute regions were determined by team locations.

Special consideration was given to models trained on custom hardware, such as BLOOM's use of the Jean Zay supercomputer in France. In these cases, private infrastructure calculations incorporated carbon efficiency (kg/kWh) and offset percentages.

The study evaluated 52 models in total: 36 industry language models (2018–25), eight industry vision models (2019–23), four academic language models (2020–23), and four academic vision models (2011–22), selecting particularly influential models in their respective domains.

### GitHub

**Identifying AI Projects:** In partnership with researchers from Harvard Business School, Microsoft Research, and Microsoft's AI for Good Lab, GitHub identifies public AI repositories following the methodologies of Gonzalez, Zimmerman, and Nagappan (2020) and Dohmke, Iansiti, and Richards (2023), using topic labels related to AI/ML and generative AI, respectively, along with other relevant keywords identified through snowball sampling, such as "machine learning," "deep learning," and "artificial intelligence." GitHub further augments the dataset with repositories that have a dependency on the PyTorch, TensorFlow, OpenAI, Transformers, XGBoost, scikit-learn, and SciPy libraries for Python.

**Mapping AI Projects to Geographic Areas:** Public AI projects are mapped to geographic areas using IP address geolocation to determine the mode location of a project's owners each year. Each project owner is assigned a location based on their IP address when interacting with GitHub. If a project owner changes locations within a year, the location for the project would be determined by the mode location of its owners sampled daily in the year. Additionally, the last known location of the project owner is carried forward on a daily basis even if the project owner performed no activities that day.

### Hugging Face

Hugging Face (HF) data is collected from two distinct sources:

- **Downloads data:** shared by the authors of Longpre et al. (2025)
- **Number of existing models and datasets:** publicly accessible Hugging Face (HF) repository

Longpre et al. (2025) is used because it provides the most consistent and complete information on actual downloads. Download data from HF can vary across releases depending on how parameters are handled. Longpre et al. collaborated directly with HF personnel, who confirmed that this dataset is the least noisy version available.

---

*Source: Stanford HAI — AI Index 2026 Annual Report. Licensed under Attribution-NoDerivatives 4.0 International.*
